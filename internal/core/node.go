package core

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/evanw/esbuild/pkg/api"
)

func nodeScript(path string) bool       { return filepath.Ext(path) == ".ts" || filepath.Ext(path) == ".js" }
func executableScript(path string) bool { return filepath.Ext(path) == ".py" || nodeScript(path) }

// compileNode bundles trusted ESM extensions. Module resolution is package-local;
// this is a reproducibility boundary, not a sandbox for Node built-ins.
func compileNode(root, entry, sdk string) ([]byte, error) {
	total, count := 0, 0
	var mu sync.Mutex
	plugin := api.Plugin{Name: "seven-package", Setup: func(build api.PluginBuild) {
		build.OnResolve(api.OnResolveOptions{Filter: ".*"}, func(args api.OnResolveArgs) (api.OnResolveResult, error) {
			if args.Kind == api.ResolveJSRequireCall || args.Kind == api.ResolveJSRequireResolve {
				return api.OnResolveResult{}, errors.New("ESM imports required")
			}
			if args.Path == "@seven-agent/eval" {
				return api.OnResolveResult{Path: sdk, Namespace: "seven-sdk"}, nil
			}
			if strings.HasPrefix(args.Path, "node:") {
				return api.OnResolveResult{Path: args.Path, External: true}, nil
			}
			path := args.Path
			if args.Kind != api.ResolveEntryPoint {
				if !strings.HasPrefix(path, "./") && !strings.HasPrefix(path, "../") {
					return api.OnResolveResult{}, errors.New("only SDK, node built-ins and relative imports are supported")
				}
				path = filepath.Join(args.ResolveDir, path)
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return api.OnResolveResult{}, err
			}
			for _, suffix := range []string{"", ".ts", ".js", ".json", "/index.ts", "/index.js"} {
				candidate, err := SafePath(root, rel+suffix)
				if err == nil {
					if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
						return api.OnResolveResult{Path: candidate}, nil
					}
				}
			}
			return api.OnResolveResult{}, errors.New("module must resolve inside the package")
		})
		load := func(args api.OnLoadArgs) (api.OnLoadResult, error) {
			mu.Lock()
			defer mu.Unlock()
			b, err := ReadBounded(args.Path, MaxFileBytes)
			if err != nil {
				return api.OnLoadResult{}, err
			}
			total += len(b)
			count++
			if total > 64<<20 || count > 2000 {
				return api.OnLoadResult{}, errors.New("module graph exceeds limits")
			}
			content := string(b)
			loader := api.LoaderJS
			switch filepath.Ext(args.Path) {
			case ".ts":
				loader = api.LoaderTS
			case ".json":
				loader = api.LoaderJSON
			}
			return api.OnLoadResult{Contents: &content, Loader: loader, ResolveDir: filepath.Dir(args.Path)}, nil
		}
		build.OnLoad(api.OnLoadOptions{Filter: ".*", Namespace: "file"}, load)
		build.OnLoad(api.OnLoadOptions{Filter: ".*", Namespace: "seven-sdk"}, load)
	}}
	result := api.Build(api.BuildOptions{EntryPoints: []string{entry}, Bundle: true, Write: false, Platform: api.PlatformNode, Format: api.FormatESModule, Target: api.ES2022, Plugins: []api.Plugin{plugin}, LogLevel: api.LogLevelSilent, TsconfigRaw: `{"compilerOptions":{"useDefineForClassFields":true}}`})
	if len(result.Errors) > 0 || len(result.OutputFiles) != 1 {
		return nil, errors.New("Node extension compilation failed; check ESM imports and package boundaries")
	}
	for _, w := range result.Warnings {
		if w.ID == "unsupported-dynamic-import" || w.ID == "unsupported-require-call" {
			return nil, errors.New("dynamic module paths are unsupported")
		}
	}
	if len(result.OutputFiles[0].Contents) > 8<<20 {
		return nil, errors.New("compiled extension exceeds limits")
	}
	return result.OutputFiles[0].Contents, nil
}

func (w Worker) nodeCommand(ctx context.Context, params map[string]any) (*exec.Cmd, func(), error) {
	node := w.Node
	if node == "" {
		node = "node"
	}
	if _, err := exec.LookPath(node); err != nil {
		return nil, nil, errors.New("Node runtime unavailable; configure SEVEN_EVAL_NODE")
	}
	path, _ := params["path"].(string)
	runtime := filepath.Join(w.Home, "sdk/typescript/dist/worker.mjs")
	cleanup := func() {}
	if w.NodeSnapshot != "" {
		rel, err := filepath.Rel(w.Package, path)
		if err != nil {
			return nil, nil, err
		}
		runtime = filepath.Join(w.NodeSnapshot, "worker.mjs")
		path = filepath.Join(w.NodeSnapshot, Hash([]byte(filepath.ToSlash(rel)))+".mjs")
		for _, file := range []string{runtime, path} {
			b, err := ReadBounded(file, MaxFileBytes)
			if err != nil {
				return nil, nil, err
			}
			if w.NodeHashes["node/"+filepath.Base(file)] != Hash(b) {
				return nil, nil, errors.New("Node snapshot integrity failure")
			}
		}
	} else {
		root := w.Package
		if root == "" {
			var err error
			root, err = FindPackage(filepath.Dir(path))
			if err != nil {
				return nil, nil, err
			}
		}
		b, err := compileNode(root, path, filepath.Join(w.Home, "sdk/typescript/src/index.ts"))
		if err != nil {
			return nil, nil, err
		}
		temp, err := os.MkdirTemp("", "seven-node-")
		if err != nil {
			return nil, nil, err
		}
		cleanup = func() { os.RemoveAll(temp) }
		path = filepath.Join(temp, "extension.mjs")
		if err = os.WriteFile(path, b, 0600); err != nil {
			cleanup()
			return nil, nil, err
		}
	}
	params["path"] = path
	return exec.CommandContext(ctx, node, runtime), cleanup, nil
}

func (e *Engine) freezeNode(ctx context.Context, r *Run) error {
	refs := append(append(append([]string{}, r.Effective.Evaluators...), r.Effective.Processors...), r.Effective.Conclusions...)
	if r.Effective.Plan.Subject != "" {
		refs = append(refs, r.Effective.Plan.Subject)
	}
	selected := map[string]bool{}
	for _, ref := range refs {
		selected[ref] = true
	}
	var assets []Asset
	for _, a := range r.Effective.Assets {
		if a.Source == "local" && nodeScript(a.Path) && selected[a.Kind+":"+a.ID] {
			assets = append(assets, a)
		}
	}
	if len(assets) == 0 {
		return nil
	}
	id := r.ID
	if r.Effective.Plan.Mode == "stream" {
		id = NewID()
		frozen := *r
		frozen.ID = id
		if err := e.snapshot(&frozen); err != nil {
			return err
		}
		r.Effective.Files = frozen.Effective.Files
	}
	base := filepath.Join(e.Store.Root, "runs", id, "runtime-node")
	if err := os.MkdirAll(base, 0700); err != nil {
		return err
	}
	total := 0
	save := func(name string, b []byte) error {
		total += len(b)
		if total > 64<<20 {
			return errors.New("Node snapshot exceeds 64 MiB")
		}
		if err := os.WriteFile(filepath.Join(base, name), b, 0600); err != nil {
			return err
		}
		r.Effective.Files["node/"+name] = Hash(b)
		return nil
	}
	runtime, err := ReadBounded(filepath.Join(e.Worker.Home, "sdk/typescript/dist/worker.mjs"), MaxFileBytes)
	if err != nil {
		return errors.New("Node SDK is not built; run make setup/build")
	}
	if err = save("worker.mjs", runtime); err != nil {
		return err
	}
	for _, p := range []string{"src/index.ts", "pnpm-lock.yaml"} {
		b, err := ReadBounded(filepath.Join(e.Worker.Home, "sdk/typescript", p), MaxFileBytes)
		if err != nil {
			return err
		}
		if err = save(filepath.Base(p), b); err != nil {
			return err
		}
	}
	compileRoot := filepath.Join(e.Store.Root, "runs", id, "source")
	for _, a := range assets {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		path, err := SafePath(compileRoot, a.Path)
		if err != nil {
			return err
		}
		b, err := compileNode(compileRoot, path, filepath.Join(base, "index.ts"))
		if err != nil {
			return err
		}
		if err = save(Hash([]byte(a.Path))+".mjs", b); err != nil {
			return err
		}
	}
	node := e.Worker.Node
	if node == "" {
		node = "node"
	}
	version, err := exec.CommandContext(ctx, node, "--version").Output()
	if err != nil {
		return errors.New("Node runtime unavailable")
	}
	r.Effective.NodeRuntime = &NodeRuntime{SnapshotID: id, Version: strings.TrimSpace(string(version))}
	return nil
}

func (e *Engine) workerFor(r *Run) Worker {
	w := e.Worker
	w.Package = e.Root
	if r.Effective.NodeRuntime != nil && validID.MatchString(r.Effective.NodeRuntime.SnapshotID) {
		w.NodeSnapshot = filepath.Join(e.Store.Root, "runs", r.Effective.NodeRuntime.SnapshotID, "runtime-node")
		w.Package = filepath.Join(e.Store.Root, "runs", r.Effective.NodeRuntime.SnapshotID, "source")
		w.NodeHashes = r.Effective.Files
	}
	return w
}

func (e *Engine) extensionPath(r *Run, a Asset) (string, error) {
	root := e.Root
	if nodeScript(a.Path) && r.Effective.NodeRuntime != nil {
		root = e.workerFor(r).Package
	}
	return SafePath(root, a.Path)
}

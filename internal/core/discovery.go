package core

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"seven-agent-eval/protocol"
	"sort"
	"strings"
	"time"
)

var validID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

func FindPackage(start string) (string, error) {
	p, e := filepath.Abs(start)
	if e != nil {
		return "", e
	}
	for {
		if _, e := os.Stat(filepath.Join(p, "evaluation.yaml")); e == nil {
			return p, nil
		}
		parent := filepath.Dir(p)
		if parent == p {
			break
		}
		p = parent
	}
	// Bounded workspace fallback. Multiple packages require explicit selection.
	matches, _ := filepath.Glob(filepath.Join(start, "evals", "*", "evaluation.yaml"))
	if len(matches) == 1 {
		return filepath.Abs(filepath.Dir(matches[0]))
	}
	if len(matches) > 1 {
		return "", errors.New("multiple packages found; use --package")
	}
	return "", errors.New("evaluation.yaml not found; use init or --package")
}
func Discover(ctx context.Context, root string, worker Worker) (Discovery, error) {
	var d Discovery
	path, e := SafePath(root, "evaluation.yaml")
	if e != nil {
		return d, e
	}
	if e = ReadDocument(path, &d.Package); e != nil {
		return d, e
	}
	if e = protocol.Validate("package", d.Package); e != nil {
		return d, e
	}
	d.Assets = []Asset{
		{Kind: "preset", ID: "builtin.basic@1", Version: "1", Source: "builtin", Path: "builtin/basic", Status: "valid"},
		{Kind: "evaluator", ID: "basic", Version: "1", Source: "builtin", Path: "builtin/basic", Status: "valid"},
		{Kind: "processor", ID: "basic-summary", Version: "1", Source: "builtin", Path: "builtin/basic", Status: "valid"},
		{Kind: "conclusion", ID: "basic-conclusion", Version: "1", Source: "builtin", Path: "builtin/basic", Status: "valid"},
	}
	kinds := map[string]string{"plans": "plan", "datasets": "dataset", "adapters": "adapter", "evaluators": "evaluator", "processors": "processor", "conclusions": "conclusion", "catalogs": "catalog"}
	for _, dir := range []string{"plans", "datasets", "adapters", "evaluators", "processors", "conclusions", "catalogs"} {
		base := filepath.Join(root, dir)
		if _, e := os.Lstat(base); os.IsNotExist(e) {
			continue
		}
		count := 0
		err := filepath.WalkDir(base, func(p string, de fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			count++
			if count > 2000 {
				return errors.New("too many package files")
			}
			rel, _ := filepath.Rel(root, p)
			if de.Type()&os.ModeSymlink != 0 {
				d.Assets = append(d.Assets, Asset{Kind: kinds[dir], ID: de.Name(), Path: filepath.ToSlash(rel), Source: "local", Status: "invalid", Message: "symlink rejected"})
				return nil
			}
			name := de.Name()
			if p != base && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "node_modules" || name == "dist" || name == "build" || name == "tests") {
				if de.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if de.IsDir() {
				if strings.Count(rel, string(filepath.Separator)) > 4 {
					return filepath.SkipDir
				}
				return nil
			}
			ext := filepath.Ext(name)
			if dir == "datasets" && name != "cases.jsonl" && name != "dataset.yaml" {
				return nil
			}
			if dir != "datasets" && ext != ".yaml" && ext != ".yml" && ext != ".json" && ext != ".py" && ext != ".ts" && ext != ".js" {
				return nil
			}
			id := strings.ReplaceAll(strings.TrimSuffix(name, ext), "_", "-")
			if name == "plan.yaml" || dir == "datasets" {
				id = filepath.Base(filepath.Dir(p))
			}
			a := Asset{Kind: kinds[dir], ID: id, Path: filepath.ToSlash(rel), Version: "0.1.0", Source: "local", Status: "valid", Descriptor: map[string]any{}}
			b, err := ReadBounded(p, MaxFileBytes)
			if err == nil {
				a.Descriptor["digest"] = Hash(b)
			}
			switch {
			case ext == ".py":
				var desc map[string]any
				c, cancel := context.WithTimeout(ctx, 10*time.Second)
				err = worker.Call(c, "describe", map[string]any{"path": p}, &desc)
				cancel()
				if err == nil {
					err = protocol.Validate("descriptor", desc)
				}
				if err == nil {
					expected := a.Kind
					if expected == "adapter" {
						expected = "subject"
					}
					if desc["kind"] != expected {
						err = errors.New("descriptor kind does not match directory")
					} else {
						a.ID = desc["id"].(string)
						a.Version = desc["version"].(string)
						desc["digest"] = Hash(b)
						a.Descriptor = desc
					}
				}
			case ext == ".ts" || ext == ".js":
				err = errors.New("TypeScript runtime is not available in phase 1")
			case ext == ".yaml" || ext == ".yml" || ext == ".json":
				if a.Kind == "evaluator" || a.Kind == "processor" || a.Kind == "conclusion" {
					err = errors.New("executable extensions require a Python descriptor in phase 1")
				}
				var doc map[string]any
				if err == nil {
					err = ReadDocument(p, &doc)
				}
				if err == nil {
					if v, ok := doc["id"].(string); ok {
						a.ID = v
					}
					if v, ok := doc["version"].(string); ok {
						a.Version = v
					}
					doc["digest"] = Hash(b)
					a.Descriptor = doc
					if a.Kind == "plan" {
						delete(doc, "digest")
						err = protocol.Validate("plan", doc)
						doc["digest"] = Hash(b)
					}
					if a.Kind == "dataset" {
						source, ok := doc["source"].(string)
						if !ok {
							err = errors.New("dataset.yaml requires a relative source")
						}
						if err == nil {
							_, err = SafePath(root, filepath.Join(filepath.Dir(rel), source))
						}
					}
				}
			}
			if !validID.MatchString(a.ID) {
				err = errors.New("invalid asset id")
			}
			if err != nil {
				a.Status = "invalid"
				a.Message = err.Error()
			}
			d.Assets = append(d.Assets, a)
			return nil
		})
		if err != nil {
			return d, err
		}
	}
	// A dataset manifest owns sibling cases.jsonl rather than creating a duplicate asset.
	manifests := map[string]bool{}
	for _, a := range d.Assets {
		if a.Kind == "dataset" && filepath.Base(a.Path) == "dataset.yaml" {
			manifests[filepath.Dir(a.Path)] = true
		}
	}
	filtered := d.Assets[:0]
	for _, a := range d.Assets {
		if a.Kind == "dataset" && filepath.Base(a.Path) == "cases.jsonl" && manifests[filepath.Dir(a.Path)] {
			continue
		}
		filtered = append(filtered, a)
	}
	d.Assets = filtered
	seen := map[string][]int{}
	for i, a := range d.Assets {
		key := a.Kind + ":" + a.ID
		seen[key] = append(seen[key], i)
	}
	for _, indices := range seen {
		if len(indices) > 1 {
			for _, i := range indices {
				d.Assets[i].Status = "invalid"
				d.Assets[i].Message = "duplicate asset id"
			}
		}
	}
	for i, a := range d.Assets {
		if a.Kind == "plan" && a.Status == "valid" {
			if _, err := Resolve(root, d, a.ID); err != nil {
				d.Assets[i].Status = "invalid"
				d.Assets[i].Message = err.Error()
			}
		}
	}
	sort.Slice(d.Assets, func(i, j int) bool { return d.Assets[i].Kind+":"+d.Assets[i].ID < d.Assets[j].Kind+":"+d.Assets[j].ID })
	return d, nil
}
func Lookup(d Discovery, reference, kind string) (Asset, error) {
	id := strings.TrimPrefix(reference, kind+":")
	if id == reference && strings.Contains(reference, ":") {
		return Asset{}, fmt.Errorf("expected %s reference", kind)
	}
	for _, a := range d.Assets {
		if a.Kind == kind && a.ID == id {
			if a.Status != "valid" {
				return a, fmt.Errorf("%s: %s", reference, a.Message)
			}
			return a, nil
		}
	}
	return Asset{}, fmt.Errorf("unresolved %s: %s", kind, reference)
}
func Resolve(root string, d Discovery, id string) (Effective, error) {
	var out Effective
	a, e := Lookup(d, id, "plan")
	if e != nil {
		return out, e
	}
	path, e := SafePath(root, a.Path)
	if e != nil {
		return out, e
	}
	if e = ReadDocument(path, &out.Plan); e != nil {
		return out, e
	}
	if out.Plan.Mode == "" {
		out.Plan.Mode = "replay"
	}
	if out.Plan.Preset == "" {
		out.Plan.Preset = "builtin.basic@1"
	}
	if out.Plan.TimeoutSeconds == 0 {
		out.Plan.TimeoutSeconds = 120
	}
	if _, e = Lookup(d, out.Plan.Dataset, "dataset"); e != nil {
		return out, e
	}
	if out.Plan.Mode == "invoke" {
		target, err := Lookup(d, out.Plan.Subject, "adapter")
		if err != nil {
			return out, err
		}
		if filepath.Ext(target.Path) != ".py" {
			var cfg HTTPConfig
			p, err := SafePath(root, target.Path)
			if err != nil {
				return out, err
			}
			if err = ReadDocument(p, &cfg); err != nil {
				return out, err
			}
			if cfg.Type != "http" || cfg.Endpoint == "" {
				return out, errors.New("HTTP adapter requires type:http and endpoint")
			}
		}
	}
	pick := func(ptr *[]string, fallback string) []string {
		if ptr == nil {
			return []string{fallback}
		}
		return append([]string{}, (*ptr)...)
	}
	out.Evaluators = pick(out.Plan.Evaluators, "evaluator:basic")
	out.Processors = pick(out.Plan.Processors, "processor:basic-summary")
	out.Conclusions = pick(out.Plan.Conclusions, "conclusion:basic-conclusion")
	for kind, refs := range map[string][]string{"evaluator": out.Evaluators, "processor": out.Processors, "conclusion": out.Conclusions} {
		seen := map[string]bool{}
		for _, r := range refs {
			v, err := Lookup(d, r, kind)
			if err != nil {
				return out, err
			}
			if seen[v.ID] {
				return out, errors.New("duplicate stage reference")
			}
			seen[v.ID] = true
		}
	}
	out.Assets = d.Assets
	out.Digest = Digest(out)
	return out, nil
}

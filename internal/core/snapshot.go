package core

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func (e *Engine) snapshot(r *Run) error {
	r.Effective.Files = map[string]string{}
	total, count := 0, 0
	copyTree := func(root, dest, prefix string) error {
		return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if p == root {
				return nil
			}
			stateAbs, _ := filepath.Abs(e.Store.Root)
			pathAbs, _ := filepath.Abs(p)
			if pathAbs == stateAbs && d.IsDir() {
				return filepath.SkipDir
			}
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "__pycache__" || name == "node_modules" || name == "dist" || name == "build" || strings.HasSuffix(name, ".seven.eval") {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.Type()&os.ModeSymlink != 0 {
				return errors.New("snapshot rejects symlinks")
			}
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			target := filepath.Join(dest, rel)
			if d.IsDir() {
				return os.MkdirAll(target, 0700)
			}
			if !d.Type().IsRegular() {
				return errors.New("snapshot requires regular files")
			}
			count++
			if count > 2000 {
				return errors.New("snapshot exceeds file count")
			}
			b, err := ReadBounded(p, MaxFileBytes)
			if err != nil {
				return err
			}
			total += len(b)
			if total > 64<<20 {
				return errors.New("snapshot exceeds 64 MiB")
			}
			if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
				return err
			}
			if err = os.WriteFile(target, b, 0600); err != nil {
				return err
			}
			r.Effective.Files[prefix+filepath.ToSlash(rel)] = Hash(b)
			return nil
		})
	}
	base := filepath.Join(e.Store.Root, "runs", r.ID)
	if err := copyTree(e.Root, filepath.Join(base, "source"), "package/"); err != nil {
		return err
	}
	if err := copyTree(filepath.Join(e.Worker.Home, "sdk/python/src"), filepath.Join(base, "runtime"), "runtime/"); err != nil {
		return err
	}
	// Runtime source and local helper code are frozen. Installed dependency versions are recorded by lock digest.
	if b, err := ReadBounded(filepath.Join(e.Worker.Home, "sdk/python/uv.lock"), MaxFileBytes); err == nil {
		r.Effective.Files["runtime/uv.lock"] = Hash(b)
	}
	return nil
}

func (e *Engine) redactForRun(r *Run, v any) any {
	value := redact(v)
	secrets := []string{}
	judgeKey := "SEVEN_EVAL_JUDGE_KEY"
	if judge, ok := r.Effective.Plan.Parameters["judge"].(map[string]any); ok {
		if key, ok := judge["auth_env"].(string); ok && key != "" {
			judgeKey = key
		}
	}
	if secret := os.Getenv(judgeKey); secret != "" {
		secrets = append(secrets, secret)
	}
	for _, a := range r.Effective.Assets {
		if key, ok := a.Descriptor["auth_env"].(string); ok && key != "" {
			if secret := os.Getenv(key); secret != "" {
				secrets = append(secrets, secret)
			}
		}
	}
	var walk func(any) any
	walk = func(x any) any {
		switch t := x.(type) {
		case string:
			for _, secret := range secrets {
				t = strings.ReplaceAll(t, secret, "[REDACTED]")
			}
			return t
		case map[string]any:
			for k, v := range t {
				t[k] = walk(v)
			}
		case []any:
			for i, v := range t {
				t[i] = walk(v)
			}
		}
		return x
	}
	return walk(value)
}

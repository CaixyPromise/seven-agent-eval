package core

import (
	"errors"
	"os"
	"path/filepath"
)

func InitPackage(root string) error {
	if _, e := os.Stat(root); e == nil {
		entries, err := os.ReadDir(root)
		if err != nil {
			return err
		}
		if len(entries) > 0 {
			return errors.New("init requires an empty or new directory")
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	files := map[string]string{
		"evaluation.yaml":             "schema_version: evaluate.package/v1\nid: example.offline\nversion: 0.1.0\nname: My evaluation\n",
		"plans/smoke.yaml":            "schema_version: evaluate.plan/v1\nid: smoke\nmode: replay\ndataset: dataset:sample\n",
		"datasets/sample/cases.jsonl": "{\"id\":\"welcome\",\"inputs\":{\"question\":\"Say hello\"},\"response\":\"Hello\",\"references\":{\"exact\":\"Hello\"}}\n",
		".gitignore":                  ".seven-evaluate/\n.venv/\n__pycache__/\n*.seven.eval\n",
	}
	for p, content := range files {
		dest := filepath.Join(root, p)
		if e := os.MkdirAll(filepath.Dir(dest), 0755); e != nil {
			return e
		}
		f, e := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if e != nil {
			return e
		}
		_, e = f.WriteString(content)
		ce := f.Close()
		if e != nil {
			return e
		}
		if ce != nil {
			return ce
		}
	}
	return nil
}

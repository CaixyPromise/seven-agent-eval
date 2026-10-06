package distribution

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func release(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "release")
	if err := os.Mkdir(root, 0755); err != nil {
		t.Fatal(err)
	}
	m := Manifest{Version: "1", Product: "seven-agent-eval", Protocol: "2.0", OS: runtime.GOOS, Arch: runtime.GOARCH, Commit: "fixture"}
	for _, p := range []string{"bin/seven-eval", "sdk/python/src/seven_evaluate/worker.py", "sdk/python/pyproject.toml", "sdk/python/uv.lock", "sdk/typescript/src/index.ts", "sdk/typescript/dist/worker.mjs", "sdk/typescript/pnpm-lock.yaml", "protocol/schemas/exchange.json", "web/dist/index.html", "requirements.txt"} {
		path := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(p), 0644); err != nil {
			t.Fatal(err)
		}
		hash, _ := digest(path)
		m.Files = append(m.Files, File{Path: p, SHA256: hash, Mode: 0644})
	}
	b, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(root, ManifestName), b, 0644); err != nil {
		t.Fatal(err)
	}
	return root
}
func TestViewerInstallOwnershipAndReinstall(t *testing.T) {
	source := release(t)
	parent := t.TempDir()
	dest := filepath.Join(parent, "install with spaces")
	r, err := Install(context.Background(), source, dest, Options{ViewerOnly: true})
	if err != nil || r.Status != "ready" {
		t.Fatalf("%+v %v", r, err)
	}
	if _, err = Doctor(context.Background(), dest, true); err != nil {
		t.Fatal(err)
	}
	if _, err = Doctor(context.Background(), dest, false); err == nil {
		t.Fatal("viewer accepted for execution")
	}
	if _, err = Install(context.Background(), source, dest, Options{ViewerOnly: true}); err == nil || !strings.Contains(err.Error(), "INSTALL_EXISTS") {
		t.Fatal(err)
	}
	outside := filepath.Join(parent, "user.seven.eval")
	os.WriteFile(outside, []byte("keep"), 0600)
	added := filepath.Join(dest, "user-empty")
	os.Mkdir(added, 0755)
	if err = Uninstall(dest); err == nil || !strings.Contains(err.Error(), "UNINSTALL_USER_CONTENT") {
		t.Fatal(err)
	}
	os.Remove(added)
	owned := filepath.Join(dest, "web/dist/index.html")
	original, _ := os.ReadFile(owned)
	os.WriteFile(owned, []byte("user edited"), 0644)
	if err = Uninstall(dest); err == nil {
		t.Fatal("removed modified resource")
	}
	os.WriteFile(owned, original, 0644)
	if err = Uninstall(dest); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("installation retained")
	}
	if b, _ := os.ReadFile(outside); string(b) != "keep" {
		t.Fatal("external data changed")
	}
	if _, err = Install(context.Background(), source, dest, Options{ViewerOnly: true}); err != nil {
		t.Fatal(err)
	}
	if err = Uninstall(dest); err != nil {
		t.Fatal(err)
	}
}
func TestResourceFailureAndPathSafety(t *testing.T) {
	root := release(t)
	if err := Outside(root, filepath.Join(root, "new", "data")); err == nil {
		t.Fatal("in-install writes accepted")
	}
	parent := t.TempDir()
	link := filepath.Join(parent, "alias")
	os.Symlink(root, link)
	if err := Outside(root, filepath.Join(link, "future")); err == nil {
		t.Fatal("symlink write accepted")
	}
	os.WriteFile(filepath.Join(root, "requirements.txt"), []byte("changed"), 0644)
	if _, err := Validate(root); err == nil || !strings.Contains(err.Error(), "INSTALL_RESOURCES_CHANGED") {
		t.Fatal(err)
	}
	root = release(t)
	var m Manifest
	readJSON(filepath.Join(root, ManifestName), &m)
	m.Files[0].Path = "../escape"
	b, _ := json.Marshal(m)
	os.WriteFile(filepath.Join(root, ManifestName), b, 0644)
	if _, err := Validate(root); err == nil {
		t.Fatal("traversal accepted")
	}
	root = release(t)
	readJSON(filepath.Join(root, ManifestName), &m)
	m.Arch = "unsupported"
	b, _ = json.Marshal(m)
	os.WriteFile(filepath.Join(root, ManifestName), b, 0644)
	if _, err := Validate(root); err == nil || !strings.Contains(err.Error(), "INSTALL_PLATFORM_MISMATCH") {
		t.Fatal(err)
	}
	root = release(t)
	fileParent := filepath.Join(t.TempDir(), "file")
	os.WriteFile(fileParent, []byte("preserve"), 0600)
	if _, err := Install(context.Background(), root, filepath.Join(fileParent, "child"), Options{ViewerOnly: true}); err == nil || !strings.Contains(err.Error(), "INSTALL_PARENT_UNAVAILABLE") {
		t.Fatal(err)
	}
}
func TestMissingRuntimeAndUnownedUninstall(t *testing.T) {
	ctx := context.Background()
	root := release(t)
	if _, err := Python(ctx, "/seven-eval-missing-runtime", false); err == nil {
		t.Fatal("missing Python accepted")
	}
	if _, err := Node(ctx, "/seven-eval-missing-runtime"); err == nil {
		t.Fatal("missing Node accepted")
	}
	if _, err := Install(ctx, root, filepath.Join(t.TempDir(), "new"), Options{}); err == nil || !strings.Contains(err.Error(), "PYTHON_REQUIRED") {
		t.Fatal(err)
	}
	if err := Uninstall(root); err == nil {
		t.Fatal("unowned release deleted")
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatal(err)
	}
}

func TestFailedInstallRetainsExplicitlyOwnedResources(t *testing.T) {
	root := release(t)
	python := filepath.Join(t.TempDir(), "python")
	// Version probe succeeds; venv creation fails, without ever running user code.
	if err := os.WriteFile(python, []byte("#!/bin/sh\nif [ \"$1\" = \"-B\" ]; then echo 3.11.0; exit 0; fi\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "failed")
	if _, err := Install(context.Background(), root, dest, Options{Python: python}); err == nil || !strings.Contains(err.Error(), "PYTHON_ENV_FAILED") {
		t.Fatal(err)
	}
	var r Receipt
	if err := readJSON(filepath.Join(dest, receiptName), &r); err != nil || r.Status != "failed" {
		t.Fatalf("%+v %v", r, err)
	}
	if err := Uninstall(dest); err != nil {
		t.Fatal(err)
	}
}

func TestProbeOutputIsBounded(t *testing.T) {
	var output probeOutput
	if _, err := output.Write(make([]byte, 4096)); err != nil {
		t.Fatal(err)
	}
	if _, err := output.Write([]byte("more")); err == nil || output.Len() != 4096 {
		t.Fatal("probe exceeded bound")
	}
}

func TestEmptyFailedInstallationCanBeRemoved(t *testing.T) {
	root := t.TempDir()
	owned := filepath.Join(root, "failed")
	if err := os.Mkdir(owned, 0755); err != nil {
		t.Fatal(err)
	}
	var err error
	owned, err = filepath.EvalSymlinks(owned)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeReceipt(owned, Receipt{Version: "1", Prefix: owned, Status: "failed", Files: []File{}}); err != nil {
		t.Fatal(err)
	}
	if err := Uninstall(owned); err != nil {
		t.Fatal(err)
	}
}

func TestReadOnlyParentIsNotModified(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses directory permission checks")
	}
	source := release(t)
	parent := t.TempDir()
	if err := os.Chmod(parent, 0555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(parent, 0755)
	dest := filepath.Join(parent, "install")
	if _, err := Install(context.Background(), source, dest, Options{ViewerOnly: true}); err == nil || !strings.Contains(err.Error(), "INSTALL_NOT_WRITABLE") {
		t.Fatal(err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("failed install created data")
	}
}

func TestBroadUninstallTargetsAlwaysRejected(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{home, string(filepath.Separator)} {
		if err := Uninstall(path); err == nil || !strings.Contains(err.Error(), "UNINSTALL_UNSAFE") {
			t.Fatal(path, err)
		}
	}
}

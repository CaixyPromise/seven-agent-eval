// Package distribution manages an explicit, self-contained installation directory.
// It never edits PATH, shell profiles, or user Evaluation Packages.
package distribution

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const ManifestName = "release.json"
const receiptName = ".seven-installation.json"

// Managed recognizes an installed directory even if its release manifest was lost.
func Managed(home string) bool {
	for _, name := range []string{ManifestName, receiptName} {
		if _, err := os.Lstat(filepath.Join(home, name)); err == nil {
			return true
		}
	}
	return false
}

type File struct {
	Path      string `json:"path"`
	SHA256    string `json:"sha256,omitempty"`
	Mode      uint32 `json:"mode"`
	Link      string `json:"link,omitempty"`
	Directory bool   `json:"directory,omitempty"`
}
type Manifest struct {
	Version  string `json:"version"`
	Product  string `json:"product"`
	Protocol string `json:"protocol"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Commit   string `json:"commit"`
	Files    []File `json:"files"`
}
type Receipt struct {
	Version string `json:"version"`
	Prefix  string `json:"prefix"`
	Status  string `json:"status"`
	Mode    string `json:"mode"`
	Files   []File `json:"files"`
}
type Options struct {
	Python     string
	ViewerOnly bool
}
type Health struct {
	Home      string `json:"home"`
	Platform  string `json:"platform"`
	Resources string `json:"resources"`
	Python    string `json:"python,omitempty"`
	Node      string `json:"node,omitempty"`
	Mode      string `json:"mode"`
}

func readJSON(path string, target any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 8<<20+1))
	if err != nil {
		return err
	}
	if len(b) > 8<<20 {
		return errors.New("INSTALL_METADATA_INVALID: metadata exceeds 8 MiB")
	}
	return json.Unmarshal(b, target)
}
func writeReceipt(root string, receipt Receipt) error {
	b, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, receiptName), b, 0600)
}
func validPath(path string) bool {
	return path != "" && !filepath.IsAbs(path) && filepath.ToSlash(filepath.Clean(path)) == path && path != "." && path != ".." && !strings.HasPrefix(path, "../") && !strings.Contains(path, "\\")
}
func digest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func contained(root, path string) bool {
	rel, e := filepath.Rel(root, path)
	return e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// Outside rejects writes inside the tool's installation, including through symlinks.
func Outside(home, path string) error {
	if !Managed(home) {
		return nil
	}
	root, err := filepath.EvalSymlinks(home)
	if err != nil {
		return err
	}
	root, _ = filepath.Abs(root)
	p, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	// Resolve the nearest existing parent, so a future directory cannot bypass the guard.
	tail := []string{}
	current := p
	for {
		resolved, e := filepath.EvalSymlinks(current)
		if e == nil {
			p = resolved
			for i := len(tail) - 1; i >= 0; i-- {
				p = filepath.Join(p, tail[i])
			}
			break
		}
		if !os.IsNotExist(e) {
			return e
		}
		tail = append(tail, filepath.Base(current))
		next := filepath.Dir(current)
		if next == current {
			return e
		}
		current = next
	}
	if contained(root, p) {
		return errors.New("INSTALL_DATA_LOCATION: put Packages, state and exports outside the installation directory")
	}
	return nil
}

// Validate verifies every declared resource before installation or execution.
func Validate(home string) (Manifest, error) {
	var m Manifest
	if err := readJSON(filepath.Join(home, ManifestName), &m); err != nil {
		return m, fmt.Errorf("INSTALL_RESOURCES_MISSING: release.json unavailable; extract the complete release archive: %w", err)
	}
	if m.Version != "1" || m.Product != "seven-agent-eval" || m.Protocol != "2.0" || len(m.Files) == 0 || len(m.Files) > 50000 {
		return m, errors.New("INSTALL_METADATA_INVALID: invalid release manifest")
	}
	if m.OS != runtime.GOOS || m.Arch != runtime.GOARCH {
		return m, fmt.Errorf("INSTALL_PLATFORM_MISMATCH: archive is %s/%s; runtime is %s/%s", m.OS, m.Arch, runtime.GOOS, runtime.GOARCH)
	}
	seen := map[string]bool{}
	for _, f := range m.Files {
		if !validPath(f.Path) || seen[f.Path] || f.Link != "" || f.Directory || len(f.SHA256) != 64 || f.Mode&^0777 != 0 {
			return m, errors.New("INSTALL_METADATA_INVALID: invalid file entry")
		}
		seen[f.Path] = true
		p := filepath.Join(home, filepath.FromSlash(f.Path))
		info, e := os.Lstat(p)
		if e != nil || !info.Mode().IsRegular() {
			return m, fmt.Errorf("INSTALL_RESOURCES_MISSING: %s missing or not a regular file; re-extract release", f.Path)
		}
		resolved, e := filepath.EvalSymlinks(p)
		root, re := filepath.EvalSymlinks(home)
		if e != nil || re != nil || !contained(root, resolved) {
			return m, errors.New("INSTALL_METADATA_INVALID: resource escapes installation")
		}
		h, e := digest(p)
		if e != nil || h != f.SHA256 || uint32(info.Mode().Perm()) != f.Mode {
			return m, fmt.Errorf("INSTALL_RESOURCES_CHANGED: %s does not match release; re-extract release", f.Path)
		}
	}
	for _, p := range []string{"bin/seven-eval", "sdk/python/src/seven_evaluate/worker.py", "sdk/python/pyproject.toml", "sdk/python/uv.lock", "sdk/typescript/src/index.ts", "sdk/typescript/dist/worker.mjs", "sdk/typescript/pnpm-lock.yaml", "protocol/schemas/exchange.json", "web/dist/index.html", "requirements.txt"} {
		if !seen[p] {
			return m, fmt.Errorf("INSTALL_RESOURCES_MISSING: %s not declared", p)
		}
	}
	return m, nil
}

type probeOutput struct{ bytes.Buffer }

func (b *probeOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 4096 {
		return 0, errors.New("runtime probe output exceeds limit")
	}
	return b.Buffer.Write(p)
}

func command(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	// These probes produce only versions/boolean results, not upstream or user data.
	var output probeOutput
	cmd.Stdout = &output
	cmd.Stderr = io.Discard
	cmd.WaitDelay = 2 * time.Second
	err := cmd.Run()
	return strings.TrimSpace(output.String()), err
}

// Python probes the wheelhouse's Python ABI, optionally checking its dependencies.
func Python(ctx context.Context, path string, dependencies bool) (string, error) {
	code := "import sys; assert sys.version_info[:2] == (3,11); print(sys.version.split()[0])"
	if dependencies {
		code = "import sys; assert sys.version_info[:2] == (3,11); import jsonschema; print(sys.version.split()[0])"
	}
	v, e := command(ctx, path, "-B", "-c", code)
	if e != nil {
		return "", errors.New("PYTHON_UNAVAILABLE: execution requires Python 3.11 and locked jsonschema dependencies; install with --python /path/to/python3.11 or use --viewer-only")
	}
	return v, nil
}

// Node validates the optional extension runtime without loading user extensions.
func Node(ctx context.Context, path string) (string, error) {
	if path == "" {
		path = "node"
	}
	v, e := command(ctx, path, "-e", "const [a,b]=process.versions.node.split('.').map(Number); if(a<20 || (a===20 && b<19)) process.exit(1); console.log(process.versions.node)")
	if e != nil {
		return "", errors.New("NODE_UNAVAILABLE: TS/JS extensions require Node >=20.19; set SEVEN_EVAL_NODE to its executable (Python-only execution and viewing do not require Node)")
	}
	return v, nil
}

// Doctor distinguishes resource-only viewing from execution readiness.
func Doctor(ctx context.Context, home string, viewer bool) (Health, error) {
	h := Health{Home: home, Platform: runtime.GOOS + "/" + runtime.GOARCH, Mode: "viewer"}
	if _, err := Validate(home); err != nil {
		return h, err
	}
	h.Resources = "verified"
	if viewer {
		return h, nil
	}
	var receipt Receipt
	if err := readJSON(filepath.Join(home, receiptName), &receipt); err != nil || receipt.Status != "ready" || receipt.Mode != "execution" {
		return h, errors.New("INSTALL_NOT_EXECUTION_READY: run install into a new directory without --viewer-only")
	}
	root, _ := filepath.Abs(home)
	root, _ = filepath.EvalSymlinks(root)
	if receipt.Prefix != root {
		return h, errors.New("INSTALL_RELOCATED: Python environment belongs to its original install location; reinstall in the final directory")
	}
	python := os.Getenv("SEVEN_EVAL_PYTHON")
	if python == "" {
		python = filepath.Join(home, "sdk/python/.venv/bin/python")
	}
	v, e := Python(ctx, python, true)
	if e != nil {
		return h, e
	}
	h.Python = v
	h.Mode = "execution"
	if v, e = Node(ctx, os.Getenv("SEVEN_EVAL_NODE")); e == nil {
		h.Node = v
	} else {
		h.Node = "unavailable (optional for Python-only plans)"
	}
	return h, nil
}
func inventory(root string) ([]File, error) {
	files := []File{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if p == root {
			return nil
		}
		rel, e := filepath.Rel(root, p)
		if e != nil {
			return e
		}
		if rel == receiptName {
			return nil
		}
		if len(files) >= 50000 {
			return errors.New("installation file count exceeds limit")
		}
		info, e := d.Info()
		if e != nil {
			return e
		}
		f := File{Path: filepath.ToSlash(rel), Mode: uint32(info.Mode().Perm())}
		if info.IsDir() {
			f.Directory = true
		} else if info.Mode()&os.ModeSymlink != 0 {
			f.Link, e = os.Readlink(p)
		} else if info.Mode().IsRegular() {
			f.SHA256, e = digest(p)
		} else {
			return errors.New("unsupported file in installation")
		}
		if e != nil {
			return e
		}
		files = append(files, f)
		return nil
	})
	return files, err
}

// Install creates a new owned prefix and, for execution, a final-location venv.
// Existing destinations are never overwritten; failed prefixes are retained.
func Install(ctx context.Context, source, destination string, opts Options) (receipt Receipt, err error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	m, err := Validate(source)
	if err != nil {
		return receipt, err
	}
	dest, err := filepath.Abs(destination)
	if err != nil {
		return receipt, err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(dest))
	if err != nil {
		return receipt, errors.New("INSTALL_PARENT_UNAVAILABLE: create a writable parent directory first")
	}
	if info, e := os.Stat(parent); e != nil || !info.IsDir() {
		return receipt, errors.New("INSTALL_PARENT_UNAVAILABLE: parent must be an existing writable directory, not a file")
	}
	dest = filepath.Join(parent, filepath.Base(dest))
	if err = Outside(source, dest); err != nil {
		return receipt, err
	}
	userHome, _ := os.UserHomeDir()
	if dest == string(filepath.Separator) || dest == userHome {
		return receipt, errors.New("INSTALL_DESTINATION_UNSAFE: choose a new dedicated child directory")
	}
	if _, e := os.Lstat(dest); !os.IsNotExist(e) {
		return receipt, errors.New("INSTALL_EXISTS: destination already exists; choose a new directory or explicitly uninstall the previous installation")
	}
	if !opts.ViewerOnly {
		if opts.Python == "" {
			return receipt, errors.New("PYTHON_REQUIRED: install --python /path/to/python3.11 DIRECTORY, or install --viewer-only DIRECTORY")
		}
		if _, err = Python(ctx, opts.Python, false); err != nil {
			return receipt, err
		}
		opts.Python, err = exec.LookPath(opts.Python)
		if err != nil {
			return receipt, err
		}
		opts.Python, err = filepath.Abs(opts.Python)
		if err != nil {
			return receipt, err
		}
	}
	if err = os.Mkdir(dest, 0755); err != nil {
		return receipt, fmt.Errorf("INSTALL_NOT_WRITABLE: cannot create dedicated installation directory: %w", err)
	}
	receipt = Receipt{Version: "1", Prefix: dest, Status: "installing", Mode: "viewer"}
	if !opts.ViewerOnly {
		receipt.Mode = "execution"
	}
	defer func() {
		if err != nil {
			receipt.Status = "failed"
		}
		files, e := inventory(dest)
		if e != nil {
			if err == nil {
				err = e
			}
			return
		}
		receipt.Files = files
		if e = writeReceipt(dest, receipt); e != nil && err == nil {
			err = e
		}
	}()
	if err = writeReceipt(dest, receipt); err != nil {
		return receipt, err
	}
	entries := append(append([]File{}, m.Files...), File{Path: ManifestName, Mode: 0644})
	for _, f := range entries {
		if ctx.Err() != nil {
			return receipt, ctx.Err()
		}
		p := filepath.Join(dest, filepath.FromSlash(f.Path))
		if err = os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			return receipt, err
		}
		var in *os.File
		in, err = os.Open(filepath.Join(source, filepath.FromSlash(f.Path)))
		if err != nil {
			return receipt, err
		}
		var out *os.File
		out, err = os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, os.FileMode(f.Mode))
		if err == nil {
			_, err = io.Copy(out, in)
			closeErr := out.Close()
			if err == nil {
				err = closeErr
			}
		}
		in.Close()
		if err != nil {
			return receipt, err
		}
	}
	if !opts.ViewerOnly {
		venv := filepath.Join(dest, "sdk/python/.venv")
		cmd := exec.CommandContext(ctx, opts.Python, "-m", "venv", venv)
		cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		cmd.WaitDelay = 2 * time.Second
		if e := cmd.Run(); e != nil {
			return receipt, errors.New("PYTHON_ENV_FAILED: Python 3.11 must include venv/ensurepip; incomplete owned installation retained for explicit uninstall")
		}
		python := filepath.Join(venv, "bin/python")
		cmd = exec.CommandContext(ctx, python, "-m", "pip", "--isolated", "install", "--no-index", "--only-binary=:all:", "--find-links", filepath.Join(dest, "wheelhouse"), "--require-hashes", "-r", filepath.Join(dest, "requirements.txt"))
		cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1", "PIP_DISABLE_PIP_VERSION_CHECK=1")
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		cmd.WaitDelay = 2 * time.Second
		if e := cmd.Run(); e != nil {
			return receipt, errors.New("PYTHON_DEPENDENCIES_FAILED: bundled locked wheels could not install; check platform/Python 3.11 and disk space; incomplete owned installation retained")
		}
		if _, err = Python(ctx, python, true); err != nil {
			return receipt, err
		}
	}
	if _, err = Validate(dest); err != nil {
		return receipt, err
	}
	receipt.Status = "ready"
	return receipt, nil
}

// Uninstall removes only an exact, unchanged owned installation directory.
func Uninstall(destination string) error {
	root, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	info, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("UNINSTALL_UNSAFE: target must be the exact installation directory, not a symlink")
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	userHome, _ := os.UserHomeDir()
	if resolved, e := filepath.EvalSymlinks(userHome); e == nil {
		userHome = resolved
	}
	if root == string(filepath.Separator) || root == userHome {
		return errors.New("UNINSTALL_UNSAFE: root and user home are never uninstall targets")
	}
	var receipt Receipt
	if err = readJSON(filepath.Join(root, receiptName), &receipt); err != nil {
		return errors.New("UNINSTALL_NOT_OWNED: no installation receipt; nothing removed")
	}
	if receipt.Version != "1" || receipt.Prefix != root || (len(receipt.Files) == 0 && receipt.Status != "failed") {
		return errors.New("UNINSTALL_NOT_OWNED: receipt does not identify this installation; nothing removed")
	}
	actual, err := inventory(root)
	if err != nil {
		return err
	}
	a, _ := json.Marshal(actual)
	b, _ := json.Marshal(receipt.Files)
	if string(a) != string(b) {
		return errors.New("UNINSTALL_USER_CONTENT: added or changed files found; installation preserved. Move your files elsewhere or restore owned files before uninstall")
	}
	return os.RemoveAll(root)
}

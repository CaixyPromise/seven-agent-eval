package core

import (
	"archive/zip"
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"seven-agent-eval/protocol"
	"sort"
	"strings"
)

const MaxBundleBytes int64 = 4 << 30
const MaxPartBytes = 16 << 20
const MaxBundleEntries = 4096

var partName = regexp.MustCompile(`^(inputs|results)/part-[0-9]{6}\.jsonl$`)

func Export(r Run, dest string) error {
	if !strings.HasSuffix(dest, ".seven.eval") {
		return errors.New("output must end in .seven.eval")
	}
	if r.Status == "running" || r.Status == "processing" {
		return errors.New("finish or cancel before exporting")
	}
	if err := ValidateRun(r); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(dest), ".seven-eval-export-")
	if err != nil {
		return err
	}
	defer f.Close()
	defer os.Remove(f.Name())
	zw := zip.NewWriter(f)
	defer zw.Close()
	manifest := Manifest{Format: "seven.eval", SchemaVersion: Version, RunID: r.ID, Status: r.Status, Files: map[string]string{}, Scope: r.Scope}
	var total int64
	write := func(name string, b []byte) error {
		total += int64(len(b))
		if total > MaxBundleBytes || len(b) > MaxPartBytes || len(manifest.Files) >= MaxBundleEntries-1 {
			return errors.New("bundle limits exceeded")
		}
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		if _, err = w.Write(b); err != nil {
			return err
		}
		manifest.Files[name] = Hash(b)
		return nil
	}
	for _, item := range []struct {
		name  string
		value any
	}{{"run.json", runMetadata(r)}, {"catalogs.json", r.Catalogs}, {"effective.json", r.Effective}} {
		b, err := json.Marshal(item.value)
		if err != nil {
			return err
		}
		if len(b) > MaxMetadataBytes {
			return errors.New("metadata limit exceeded")
		}
		if err := write(item.name, b); err != nil {
			return err
		}
	}
	schema, err := protocol.Files.ReadFile("schemas/exchange.json")
	if err != nil {
		return err
	}
	if err := write("schemas/exchange.json", schema); err != nil {
		return err
	}
	report, err := Report(r)
	if err != nil {
		return err
	}
	if len(report) > MaxMetadataBytes {
		return errors.New("report metadata limit exceeded")
	}
	if err := write("report.html", report); err != nil {
		return err
	}
	parts := func(prefix string, stream func(func(any) error) error) error {
		buffer := make([]byte, 0, 1<<20)
		index := 0
		flush := func() error {
			if len(buffer) == 0 {
				return nil
			}
			index++
			err := write(fmt.Sprintf("%s/part-%06d.jsonl", prefix, index), buffer)
			buffer = buffer[:0]
			return err
		}
		err := stream(func(value any) error {
			b, err := json.Marshal(value)
			if err != nil {
				return err
			}
			if len(b)+1 > MaxCaseResultBytes {
				return errors.New("record limit exceeded")
			}
			if len(buffer)+len(b)+1 > MaxPartBytes {
				if err := flush(); err != nil {
					return err
				}
			}
			buffer = append(buffer, b...)
			buffer = append(buffer, '\n')
			return nil
		})
		if err != nil {
			return err
		}
		return flush()
	}
	if err := parts("inputs", func(emit func(any) error) error {
		return r.ForEachInput(func(c Case) error { manifest.InputCount++; return emit((&Engine{}).redactForRun(&r, c)) })
	}); err != nil {
		return err
	}
	if err := parts("results", func(emit func(any) error) error {
		return r.ForEachCase(func(c CaseResult) error { manifest.ResultCount++; return emit(c) })
	}); err != nil {
		return err
	}
	if manifest.InputCount > r.Total || manifest.ResultCount != r.Completed {
		return errors.New("bundle counts inconsistent")
	}
	b, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	w, err := zw.Create("manifest.json")
	if err != nil {
		return err
	}
	if _, err = w.Write(b); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Link(f.Name(), dest); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(dest))
}

func Import(reader io.ReaderAt, size int64) (Run, error) {
	var r Run
	if size < 0 || size > MaxBundleBytes {
		return r, errors.New("archive size limit")
	}
	z, err := zip.NewReader(reader, size)
	if err != nil {
		return r, errors.New("invalid ZIP")
	}
	if len(z.File) > MaxBundleEntries {
		return r, errors.New("too many entries")
	}
	files := map[string]*zip.File{}
	digests := map[string]string{}
	var total uint64
	for _, f := range z.File {
		n := f.Name
		if strings.ContainsAny(n, "\\:") || strings.HasPrefix(n, "/") || path.Clean(n) != n || n == ".." || strings.HasPrefix(n, "../") || !f.Mode().IsRegular() {
			return r, errors.New("unsafe archive path")
		}
		if _, ok := files[n]; ok {
			return r, errors.New("duplicate archive entry")
		}
		limit := uint64(MaxMetadataBytes)
		if partName.MatchString(n) {
			limit = MaxPartBytes
		}
		if f.UncompressedSize64 > limit {
			return r, errors.New("entry limit")
		}
		total += f.UncompressedSize64
		if total > uint64(MaxBundleBytes) {
			return r, errors.New("archive expands beyond limit")
		}
		rd, err := f.Open()
		if err != nil {
			return r, err
		}
		h := sha256.New()
		nread, err := io.Copy(h, io.LimitReader(rd, int64(f.UncompressedSize64)+1))
		rd.Close()
		if err != nil || uint64(nread) != f.UncompressedSize64 {
			return r, errors.New("corrupt archive entry")
		}
		files[n] = f
		digests[n] = hex.EncodeToString(h.Sum(nil))
	}
	read := func(name string, target any) error {
		f := files[name]
		if f == nil {
			return errors.New("required archive entry missing")
		}
		rd, err := f.Open()
		if err != nil {
			return err
		}
		defer rd.Close()
		b, err := io.ReadAll(io.LimitReader(rd, MaxMetadataBytes+1))
		if err != nil {
			return err
		}
		if len(b) > MaxMetadataBytes {
			return errors.New("metadata limit")
		}
		return decodeJSON(b, target)
	}
	var m Manifest
	if err := read("manifest.json", &m); err != nil {
		return r, err
	}
	if m.SchemaVersion != Version {
		return r, errors.New("unsupported bundle version; regenerate with current evaluator")
	}
	if err := protocol.Validate("manifest", m); err != nil {
		return r, err
	}
	if len(m.Files) != len(files)-1 {
		return r, errors.New("manifest file set mismatch")
	}
	required := map[string]bool{"run.json": true, "catalogs.json": true, "effective.json": true, "schemas/exchange.json": true, "report.html": true}
	names := map[string][]string{"inputs": {}, "results": {}}
	for name, digest := range m.Files {
		if files[name] == nil || digests[name] != digest {
			return r, errors.New("archive digest mismatch")
		}
		if required[name] {
			continue
		}
		if !partName.MatchString(name) {
			return r, errors.New("unexpected archive entry")
		}
		prefix := strings.Split(name, "/")[0]
		names[prefix] = append(names[prefix], name)
	}
	for name := range required {
		if files[name] == nil {
			return r, errors.New("archive incomplete")
		}
	}
	for prefix, parts := range names {
		sort.Strings(parts)
		for i, name := range parts {
			if name != fmt.Sprintf("%s/part-%06d.jsonl", prefix, i+1) {
				return r, errors.New("part sequence gap")
			}
		}
	}
	if err := read("run.json", &r); err != nil {
		return r, err
	}
	if len(r.Cases) > 0 {
		return r, errors.New("run metadata must not embed cases")
	}
	stream := func(prefix string, visit func([]byte) error) error {
		for _, name := range names[prefix] {
			f, err := files[name].Open()
			if err != nil {
				return err
			}
			err = func() error {
				defer f.Close()
				scan := bufio.NewScanner(f)
				scan.Buffer(make([]byte, 4096), MaxCaseResultBytes)
				for scan.Scan() {
					if len(scan.Bytes()) == 0 {
						return errors.New("empty record")
					}
					if err := visit(scan.Bytes()); err != nil {
						return err
					}
				}
				return scan.Err()
			}()
			if err != nil {
				return err
			}
		}
		return nil
	}
	r.caseSource = func(visit func(CaseResult) error) error {
		return stream("results", func(b []byte) error {
			var c CaseResult
			if err := decodeJSON(b, &c); err != nil {
				return err
			}
			return visit(c)
		})
	}
	r.inputSource = func(visit func(Case) error) error {
		return stream("inputs", func(b []byte) error {
			var c Case
			if err := decodeJSON(b, &c); err != nil {
				return err
			}
			return visit(c)
		})
	}
	if r.ID != m.RunID || r.Status != m.Status || r.Status == "running" || r.Status == "processing" || r.Completed != m.ResultCount || Digest(r.Scope) != Digest(m.Scope) {
		return r, errors.New("manifest/run mismatch")
	}
	if err := ValidateRun(r); err != nil {
		return r, err
	}
	inputs := map[string]string{}
	if err := r.ForEachInput(func(c Case) error {
		if len(inputs) >= MaxCases {
			return errors.New("input count limit")
		}
		if _, ok := inputs[c.ID]; ok {
			return errors.New("duplicate input")
		}
		if err := protocol.Validate("case", c); err != nil {
			return err
		}
		inputs[c.ID] = Digest(c)
		return nil
	}); err != nil {
		return r, err
	}
	if len(inputs) != m.InputCount || len(inputs) > r.Total {
		return r, errors.New("input count mismatch")
	}
	if err := r.ForEachCase(func(c CaseResult) error {
		if inputs[c.Case.ID] != Digest(c.Case) {
			return errors.New("input/result mismatch")
		}
		return nil
	}); err != nil {
		return r, err
	}
	var catalog, effective any
	if err := read("catalogs.json", &catalog); err != nil || Digest(catalog) != Digest(r.Catalogs) {
		return r, errors.New("catalog mismatch")
	}
	if err := read("effective.json", &effective); err != nil || Digest(effective) != Digest(r.Effective) {
		return r, errors.New("effective config mismatch")
	}
	return r, nil
}

func ImportFile(p string) (Run, error) {
	f, err := os.Open(p)
	if err != nil {
		return Run{}, err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return Run{}, err
	}
	r, err := Import(f, stat.Size())
	if err != nil {
		return r, err
	}
	// Freeze the validated manifest; every subsequent part read is checked before
	// delivering records, even if the source archive was replaced after ImportFile.
	zr, err := zip.NewReader(f, stat.Size())
	if err != nil {
		return Run{}, err
	}
	var manifest Manifest
	for _, entry := range zr.File {
		if entry.Name != "manifest.json" {
			continue
		}
		rd, err := entry.Open()
		if err != nil {
			return Run{}, err
		}
		body, err := io.ReadAll(io.LimitReader(rd, MaxMetadataBytes+1))
		rd.Close()
		if err != nil {
			return Run{}, err
		}
		if err = decodeJSON(body, &manifest); err != nil {
			return Run{}, err
		}
	}
	// Reopen each iteration; no leaked file descriptor or eager Run hydration.
	parts := func(prefix string, visit func([]byte) error) error {
		z, err := zip.OpenReader(p)
		if err != nil {
			return err
		}
		defer z.Close()
		files := map[string]*zip.File{}
		for _, f := range z.File {
			if files[f.Name] != nil {
				return errors.New("duplicate archive entry")
			}
			files[f.Name] = f
		}
		names := []string{}
		for name := range manifest.Files {
			if strings.HasPrefix(name, prefix+"/") {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		for _, name := range names {
			file := files[name]
			if file == nil || file.UncompressedSize64 > MaxPartBytes {
				return errors.New("archive changed")
			}
			rd, err := file.Open()
			if err != nil {
				return err
			}
			body, err := io.ReadAll(io.LimitReader(rd, MaxPartBytes+1))
			rd.Close()
			if err != nil {
				return err
			}
			if len(body) > MaxPartBytes || Hash(body) != manifest.Files[name] {
				return errors.New("archive changed")
			}
			err = func() error {
				s := bufio.NewScanner(bytes.NewReader(body))
				s.Buffer(make([]byte, 4096), MaxCaseResultBytes)
				for s.Scan() {
					if err := visit(s.Bytes()); err != nil {
						return err
					}
				}
				return s.Err()
			}()
			if err != nil {
				return err
			}
		}
		return nil
	}
	r.caseSource = func(visit func(CaseResult) error) error {
		return parts("results", func(b []byte) error {
			var c CaseResult
			if err := decodeJSON(b, &c); err != nil {
				return err
			}
			return visit(c)
		})
	}
	r.inputSource = func(visit func(Case) error) error {
		return parts("inputs", func(b []byte) error {
			var c Case
			if err := decodeJSON(b, &c); err != nil {
				return err
			}
			return visit(c)
		})
	}
	return r, nil
}

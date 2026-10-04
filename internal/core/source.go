package core

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"seven-agent-eval/protocol"
	"strconv"
)

// SourceConfig paths are relative to the Package, never the descriptor directory.
type SourceConfig struct {
	ID       string `yaml:"id"`
	Version  string `yaml:"version"`
	Format   string `yaml:"format"`
	Path     string `yaml:"path"`
	Corpus   string `json:"corpus"`
	Queries  string `json:"queries"`
	Qrels    string `json:"qrels"`
	Run      string `json:"run"`
	Split    string `json:"split"`
	CasePath string `json:"case"`
}
type MappingConfig struct {
	ID          string            `yaml:"id"`
	Version     string            `yaml:"version"`
	Fields      map[string]any    `yaml:"fields"`
	Conversions map[string]string `yaml:"conversions"`
}

func (e *Engine) visitSource(ctx context.Context, d Discovery, p Plan, emit func(Case) error) (DatasetRevision, error) {
	rev := DatasetRevision{Files: map[string]string{}}
	a, err := Lookup(d, p.Source, "source")
	if err != nil {
		return rev, err
	}
	descriptor, err := SafePath(e.Root, a.Path)
	if err != nil {
		return rev, err
	}
	var cfg SourceConfig
	if err = ReadDocument(descriptor, &cfg); err != nil {
		return rev, err
	}
	rev.SourceID, rev.SourceVersion = a.ID, a.Version
	rev.Files[a.Path] = fmt.Sprint(a.Descriptor["digest"])
	if cfg.Format == "seven-history" {
		if p.Mode != "replay" {
			return rev, errors.New("Seven history requires replay mode")
		}
		cases, frozen, err := e.readSevenHistory(cfg, rev)
		if err != nil {
			return frozen, err
		}
		for _, c := range cases {
			if err := emit(c); err != nil {
				return frozen, err
			}
		}
		return frozen, nil
	}
	if cfg.Format == "beir" || cfg.Format == "trec" {
		if p.Mapping != "" {
			return rev, errors.New("BEIR/TREC use their standardized mapping")
		}
		return e.visitRetrievalSource(ctx, cfg, rev, p, emit)
	}
	path, err := SafePath(e.Root, cfg.Path)
	if err != nil {
		return rev, err
	}
	file, err := os.Open(path)
	if err != nil {
		return rev, err
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return rev, err
	}
	if !stat.Mode().IsRegular() || stat.Size() > MaxInputFileBytes {
		return rev, errors.New("source file exceeds input limit")
	}
	hash := sha256.New()
	limited := &io.LimitedReader{R: file, N: MaxInputFileBytes + 1}
	reader := io.TeeReader(limited, hash)
	var mapping MappingConfig
	if p.Mapping != "" {
		m, er := Lookup(d, p.Mapping, "mapping")
		if er != nil {
			return rev, er
		}
		mp, er := SafePath(e.Root, m.Path)
		if er != nil {
			return rev, er
		}
		if er = ReadDocument(mp, &mapping); er != nil {
			return rev, er
		}
		rev.MappingID, rev.MappingVersion = m.ID, m.Version
		rev.Files[m.Path] = fmt.Sprint(m.Descriptor["digest"])
	}
	count := 0
	appendRow := func(row any, line int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		value := row
		if mapping.Fields != nil {
			value, err = mapValue(mapping.Fields, row)
			if err != nil {
				return fmt.Errorf("source row %d: mapping failed: %w", line, err)
			}
		}
		obj, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("source row %d: mapped case must be object", line)
		}
		for field, kind := range mapping.Conversions {
			v, present := obj[field]
			if !present {
				continue
			}
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("source row %d field %s: conversion requires string", line, field)
			}
			if s == "" && kind != "string" {
				delete(obj, field)
				continue
			}
			switch kind {
			case "json":
				var parsed any
				err = decodeJSON([]byte(s), &parsed)
				obj[field] = parsed
			case "number":
				var n any
				err = decodeJSON([]byte(s), &n)
				if _, ok := n.(json.Number); err == nil && !ok {
					err = errors.New("not a JSON number")
				}
				obj[field] = n
			case "boolean":
				var b bool
				b, err = strconv.ParseBool(s)
				obj[field] = b
			case "string":
			default:
				return fmt.Errorf("unsupported conversion for field %s", field)
			}
			if err != nil {
				return fmt.Errorf("source row %d field %s: invalid %s", line, field, kind)
			}
		}
		if err = protocol.Validate("case", obj); err != nil {
			return fmt.Errorf("source row %d: %w", line, err)
		}
		if err = validateCaseReferences(obj); err != nil {
			return fmt.Errorf("source row %d: %w", line, err)
		}
		if p.Mode == "replay" {
			if err = validateReplayInput(obj); err != nil {
				return fmt.Errorf("source row %d: %w", line, err)
			}
		}
		b, er := json.Marshal(obj)
		if er != nil {
			return er
		}
		var c Case
		if er = decodeJSON(b, &c); er != nil {
			return er
		}
		count++
		if count > MaxCases {
			return errors.New("source exceeds 1000 cases")
		}
		return emit(c)
	}
	switch cfg.Format {
	case "jsonl":
		scan := bufio.NewScanner(reader)
		scan.Buffer(make([]byte, 4096), 1<<20)
		line := 0
		for scan.Scan() {
			line++
			if len(bytes.TrimSpace(scan.Bytes())) == 0 {
				continue
			}
			var row map[string]any
			if err = decodeJSON(scan.Bytes(), &row); err != nil {
				return rev, fmt.Errorf("source row %d: invalid JSON", line)
			}
			if err = appendRow(row, line); err != nil {
				return rev, err
			}
		}
		if err = scan.Err(); err != nil {
			return rev, err
		}
	case "csv":
		bounded := &csvRecordReader{reader: reader, remaining: inputRecordBytes}
		reader := csv.NewReader(bounded)
		headers, er := reader.Read()
		if er != nil {
			return rev, er
		}
		if reader.InputOffset() > inputRecordBytes {
			return rev, errors.New("CSV header exceeds record limit")
		}
		keys := map[string]bool{}
		for _, key := range headers {
			if key == "" || keys[key] {
				return rev, errors.New("duplicate or empty CSV header")
			}
			keys[key] = true
		}
		for {
			bounded.remaining = inputRecordBytes
			before := reader.InputOffset()
			cells, er := reader.Read()
			if er == io.EOF {
				break
			}
			if er != nil {
				return rev, er
			}
			if reader.InputOffset()-before > inputRecordBytes {
				return rev, errors.New("CSV row exceeds record limit")
			}
			row := map[string]any{}
			for i, key := range headers {
				row[key] = cells[i]
			}
			line, _ := reader.FieldPos(0)
			if err = appendRow(row, line); err != nil {
				return rev, err
			}
		}
	default:
		return rev, errors.New("unsupported source format")
	}
	if count == 0 {
		return rev, errors.New("empty source")
	}
	if limited.N == 0 {
		return rev, errors.New("source grew past size limit")
	}
	rev.Files[cfg.Path] = hex.EncodeToString(hash.Sum(nil))
	rev.Digest = Digest(rev)
	return rev, nil
}

// encoding/csv may otherwise allocate an entire unbounded multiline field. Its
// internal 4-KiB lookahead plus this resettable budget bounds allocations before
// Read returns; InputOffset enforces the exact logical-record limit afterwards.
type csvRecordReader struct {
	reader    io.Reader
	remaining int
}

func (r *csvRecordReader) Read(p []byte) (int, error) {
	if r.remaining == 0 {
		return 0, errors.New("CSV row exceeds record limit")
	}
	if len(p) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := r.reader.Read(p)
	r.remaining -= n
	return n, err
}

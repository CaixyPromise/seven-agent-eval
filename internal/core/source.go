package core

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

func (e *Engine) readSource(d Discovery, p Plan) ([]Case, DatasetRevision, error) {
	rev := DatasetRevision{Files: map[string]string{}}
	a, err := Lookup(d, p.Source, "source")
	if err != nil {
		return nil, rev, err
	}
	descriptor, err := SafePath(e.Root, a.Path)
	if err != nil {
		return nil, rev, err
	}
	var cfg SourceConfig
	if err = ReadDocument(descriptor, &cfg); err != nil {
		return nil, rev, err
	}
	rev.SourceID, rev.SourceVersion = a.ID, a.Version
	rev.Files[a.Path] = fmt.Sprint(a.Descriptor["digest"])
	if cfg.Format == "seven-history" {
		if p.Mode != "replay" {
			return nil, rev, errors.New("Seven history requires replay mode")
		}
		return e.readSevenHistory(cfg, rev)
	}
	if cfg.Format == "beir" || cfg.Format == "trec" {
		if p.Mapping != "" {
			return nil, rev, errors.New("BEIR/TREC use their standardized mapping")
		}
		return e.readRetrievalSource(cfg, rev, p)
	}
	path, err := SafePath(e.Root, cfg.Path)
	if err != nil {
		return nil, rev, err
	}
	body, err := ReadBounded(path, MaxFileBytes)
	if err != nil {
		return nil, rev, err
	}
	rev.Files[cfg.Path] = Hash(body)
	var mapping MappingConfig
	if p.Mapping != "" {
		m, er := Lookup(d, p.Mapping, "mapping")
		if er != nil {
			return nil, rev, er
		}
		mp, er := SafePath(e.Root, m.Path)
		if er != nil {
			return nil, rev, er
		}
		if er = ReadDocument(mp, &mapping); er != nil {
			return nil, rev, er
		}
		rev.MappingID, rev.MappingVersion = m.ID, m.Version
		rev.Files[m.Path] = fmt.Sprint(m.Descriptor["digest"])
	}
	cases := []Case{}
	seen := map[string]bool{}
	appendRow := func(row any, line int) error {
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
		if p.Mode == "replay" && obj["episode"] == nil {
			if _, ok := obj["response"]; !ok && obj["result"] == nil {
				return fmt.Errorf("source row %d: replay requires response", line)
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
		if seen[c.ID] {
			return fmt.Errorf("source row %d: duplicate case id", line)
		}
		seen[c.ID] = true
		cases = append(cases, c)
		if len(cases) > MaxCases {
			return errors.New("source exceeds 1000 cases")
		}
		return nil
	}
	switch cfg.Format {
	case "jsonl":
		scan := bufio.NewScanner(bytes.NewReader(body))
		scan.Buffer(make([]byte, 4096), 1<<20)
		line := 0
		for scan.Scan() {
			line++
			if len(bytes.TrimSpace(scan.Bytes())) == 0 {
				continue
			}
			var row map[string]any
			if err = decodeJSON(scan.Bytes(), &row); err != nil {
				return nil, rev, fmt.Errorf("source row %d: invalid JSON", line)
			}
			if err = appendRow(row, line); err != nil {
				return nil, rev, err
			}
		}
		if err = scan.Err(); err != nil {
			return nil, rev, err
		}
	case "csv":
		reader := csv.NewReader(bytes.NewReader(body))
		headers, er := reader.Read()
		if er != nil {
			return nil, rev, er
		}
		keys := map[string]bool{}
		for _, key := range headers {
			if key == "" || keys[key] {
				return nil, rev, errors.New("duplicate or empty CSV header")
			}
			keys[key] = true
		}
		for {
			cells, er := reader.Read()
			if er == io.EOF {
				break
			}
			if er != nil {
				return nil, rev, er
			}
			row := map[string]any{}
			for i, key := range headers {
				row[key] = cells[i]
			}
			line, _ := reader.FieldPos(0)
			if err = appendRow(row, line); err != nil {
				return nil, rev, err
			}
		}
	default:
		return nil, rev, errors.New("unsupported source format")
	}
	if len(cases) == 0 {
		return nil, rev, errors.New("empty source")
	}
	rev.Digest = Digest(struct {
		Revision DatasetRevision
		Cases    []Case
	}{rev, cases})
	return cases, rev, nil
}

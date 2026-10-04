package core

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"seven-agent-eval/protocol"
)

const inputRecordBytes = 2 << 20

// prepareInput validates every row before publishing an immutable, normalized
// execution stream. SQLite owns uniqueness checks and candidate reordering;
// neither evidence nor the ID set grows in Go memory.
func (e *Engine) prepareInput(ctx context.Context, d Discovery, r *Run) (map[string]bool, error) {
	base := filepath.Join(e.Store.Root, "runs", r.ID)
	if err := os.Mkdir(base, 0700); err != nil {
		return nil, err
	}
	published := false
	defer func() {
		if !published {
			_ = os.RemoveAll(base)
		}
	}() // Newly owned, never an existing run.
	dir, err := os.MkdirTemp(base, ".preparing-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir) // Only this call's newly created private scratch directory.
	db, err := sql.Open("sqlite", filepath.Join(dir, "index.sqlite"))
	if err != nil {
		return nil, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(`PRAGMA cache_size=-2048; PRAGMA temp_store=FILE;
 CREATE TABLE identities(kind TEXT, id TEXT, PRIMARY KEY(kind,id));
 CREATE TABLE cases(seq INTEGER PRIMARY KEY, id TEXT UNIQUE, body BLOB NOT NULL);`); err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	count := 0
	canonical := sha256.New()
	emit := func(c Case) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO identities VALUES('case',?)", c.ID); err != nil {
			return fmt.Errorf("duplicate input case id: %w", err)
		}
		if c.Episode != nil {
			if _, err := tx.ExecContext(ctx, "INSERT INTO identities VALUES('episode',?)", c.Episode.ID); err != nil {
				return errors.New("duplicate episode id")
			}
		}
		turns, err := expandEpisodes([]Case{c})
		if err != nil {
			return err
		}
		for _, turn := range turns {
			if len(r.Effective.Plan.Candidates) > 0 && turn.Metadata["episode_id"] != nil {
				return errors.New("comparison does not support shared episode environments")
			}
			raw, err := json.Marshal(turn)
			if err != nil {
				return err
			}
			if len(raw)+1 > inputRecordBytes {
				return errors.New("normalized case exceeds record limit")
			}
			// Validate expanded turns as well as the outer Episode.
			if err := protocol.Validate("case", turn); err != nil {
				return err
			}
			var obj map[string]any
			if err := decodeJSON(raw, &obj); err != nil {
				return err
			}
			if err := validateCaseReferences(obj); err != nil {
				return err
			}
			if r.Effective.Plan.Mode == "replay" {
				if err := validateReplayInput(obj); err != nil {
					return err
				}
			}
			count++
			if count > MaxCases {
				return errors.New("prepared input exceeds current case limit")
			}
			if _, err := tx.ExecContext(ctx, "INSERT INTO cases VALUES(?,?,?)", count, turn.ID, raw); err != nil {
				return fmt.Errorf("duplicate normalized case id: %w", err)
			}
			canonical.Write(raw)
			canonical.Write([]byte{'\n'})
			if id, ok := turn.Metadata["episode_id"].(string); ok {
				i := episodeIndex(r, id)
				r.Episodes[i].CaseIDs = append(r.Episodes[i].CaseIDs, turn.ID)
			}
		}
		return nil
	}
	excluded := map[string]bool{}
	if r.Effective.Plan.Source != "" {
		rev, err := e.visitSource(ctx, d, r.Effective.Plan, emit)
		if err != nil {
			return nil, err
		}
		r.DatasetRevision = &rev
		for name := range rev.Files {
			excluded[filepath.Clean(name)] = true
		}
		// Descriptor code/configuration remains available to inspection and snapshots.
		for _, a := range d.Assets {
			if a.Kind != "dataset" {
				delete(excluded, filepath.Clean(a.Path))
			}
		}
	} else {
		rel, err := datasetPath(d, r.Effective.Plan)
		if err != nil {
			return nil, err
		}
		path, err := SafePath(e.Root, rel)
		if err != nil {
			return nil, err
		}
		if err := visitJSONLCases(ctx, path, r.Effective.Plan, emit); err != nil {
			return nil, err
		}
		excluded[filepath.Clean(rel)] = true
	}
	if count == 0 {
		return nil, errors.New("empty input")
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	if len(r.Effective.Plan.Candidates) > 0 {
		if count*len(r.Effective.Plan.Candidates) > MaxCases {
			return nil, errors.New("comparison exceeds current invocation limit")
		}
		r.Comparison = &Comparison{BaselineID: r.Effective.Plan.Candidates[0].ID, InputDigest: hex.EncodeToString(canonical.Sum(nil)), CaseIDs: []string{}}
	}
	out, err := os.OpenFile(filepath.Join(dir, "cases.jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	defer out.Close()
	hash := sha256.New()
	writer := bufio.NewWriter(io.MultiWriter(out, hash))
	passes := r.Effective.Plan.Candidates
	if len(passes) == 0 {
		passes = []Candidate{{}}
	}
	for pass, candidate := range passes {
		rows, err := db.QueryContext(ctx, "SELECT body FROM cases ORDER BY seq")
		if err != nil {
			return nil, err
		}
		err = func() error {
			defer rows.Close()
			for rows.Next() {
				if err := ctx.Err(); err != nil {
					return err
				}
				var body []byte
				if err := rows.Scan(&body); err != nil {
					return err
				}
				var c Case
				if err := decodeJSON(body, &c); err != nil {
					return err
				}
				if r.Comparison != nil {
					if pass == 0 {
						r.Comparison.CaseIDs = append(r.Comparison.CaseIDs, c.ID)
					}
					c.Metadata = cloneMap(c.Metadata)
					c.Metadata["seven_eval_candidate"] = candidate.ID
					c.Metadata["seven_eval_case_id"] = c.ID
					c.ID = candidate.ID + "/" + c.ID
				}
				if err := e.preflightParameters(r, []Case{c}); err != nil {
					return err
				}
				b, err := json.Marshal(c)
				if err != nil {
					return err
				}
				if len(b)+1 > inputRecordBytes {
					return errors.New("normalized case exceeds record limit")
				}
				if _, err = writer.Write(append(b, '\n')); err != nil {
					return err
				}
				r.Total++
			}
			return rows.Err()
		}()
		if err != nil {
			return nil, err
		}
	}
	if err := writer.Flush(); err != nil {
		return nil, err
	}
	if err := out.Sync(); err != nil {
		return nil, err
	}
	if err := out.Close(); err != nil {
		return nil, err
	}
	r.DatasetDigest = hex.EncodeToString(hash.Sum(nil))
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dest := filepath.Join(base, "inputs")
	if err := os.Mkdir(dest, 0700); err != nil {
		return nil, err
	}
	if err := os.Rename(filepath.Join(dir, "cases.jsonl"), filepath.Join(dest, "cases.jsonl")); err != nil {
		return nil, err
	}
	if err := syncDirectory(dest); err != nil {
		return nil, err
	}
	if err := syncDirectory(base); err != nil {
		return nil, err
	}
	published = true
	return excluded, nil
}

func datasetPath(d Discovery, p Plan) (string, error) {
	a, err := Lookup(d, p.Dataset, "dataset")
	if err != nil {
		return "", err
	}
	rel := a.Path
	if filepath.Base(rel) == "dataset.yaml" {
		source, _ := a.Descriptor["source"].(string)
		rel = filepath.Join(filepath.Dir(rel), source)
	}
	return rel, nil
}

func visitJSONLCases(ctx context.Context, path string, p Plan, emit func(Case) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return err
	}
	if !stat.Mode().IsRegular() || stat.Size() > MaxInputFileBytes {
		return errors.New("input file exceeds current size limit")
	}
	limited := &io.LimitedReader{R: f, N: MaxInputFileBytes + 1}
	scan := bufio.NewScanner(limited)
	scan.Buffer(make([]byte, 4096), inputRecordBytes)
	for line := 1; scan.Scan(); line++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(bytes.TrimSpace(scan.Bytes())) == 0 {
			continue
		}
		var raw map[string]any
		if err := decodeJSON(scan.Bytes(), &raw); err != nil {
			return fmt.Errorf("dataset line %d: invalid JSON", line)
		}
		if err := protocol.Validate("case", raw); err != nil {
			return fmt.Errorf("dataset line %d: %w", line, err)
		}
		if err := validateCaseReferences(raw); err != nil {
			return fmt.Errorf("dataset line %d: %w", line, err)
		}
		if p.Mode == "replay" {
			if err := validateReplayInput(raw); err != nil {
				return fmt.Errorf("dataset line %d: %w", line, err)
			}
		}
		var c Case
		if err := decodeJSON(scan.Bytes(), &c); err != nil {
			return err
		}
		if err := emit(c); err != nil {
			return fmt.Errorf("dataset line %d: %w", line, err)
		}
	}
	if limited.N == 0 {
		return errors.New("input grew past size limit")
	}
	return scan.Err()
}

// openPreparedInput verifies the entire frozen file before returning its reader.
// A corrupt late row must never be detected after an earlier business call.
func (e *Engine) openPreparedInput(ctx context.Context, r Run) (*os.File, error) {
	path, err := SafePath(e.Store.Root, filepath.Join("runs", r.ID, "inputs", "cases.jsonl"))
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			f.Close()
		}
	}()
	h := sha256.New()
	scan := bufio.NewScanner(io.TeeReader(f, h))
	scan.Buffer(make([]byte, 4096), inputRecordBytes)
	n := 0
	for scan.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n++
	}
	if err := scan.Err(); err != nil {
		return nil, err
	}
	if n != r.Total || hex.EncodeToString(h.Sum(nil)) != r.DatasetDigest {
		return nil, errors.New("prepared input integrity mismatch")
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	ok = true
	return f, nil
}

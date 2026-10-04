package core

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func caseAnomaly(c CaseResult) bool {
	if c.Status != "completed" {
		return true
	}
	for _, m := range c.Metrics {
		if m.Status == "error" || (m.Status == "observed" && m.Value == false) {
			return true
		}
	}
	return false
}

func insertCase(tx *sql.Tx, id string, ordinal int, c CaseResult, body []byte) error {
	if len(body) > MaxCaseResultBytes {
		return errors.New("case result exceeds record limit")
	}
	field := func(key string) string { v, _ := c.Case.Metadata[key].(string); return v }
	_, err := tx.Exec("INSERT INTO run_cases(run_id,ordinal,body,case_id,episode,candidate,original_id,anomaly) VALUES(?,?,?,?,?,?,?,?)", id, ordinal, body, c.Case.ID, field("episode_id"), field("seven_eval_candidate"), field("seven_eval_case_id"), caseAnomaly(c))
	return err
}

// WalkCases bounds each database fetch by both record count and encoded bytes.
// The lock and query are released before visiting (workers may take time).
func (s *Store) WalkCases(id string, visit func(CaseResult) error) error {
	for offset := 0; ; {
		batch, err := s.readCaseBatch(id, offset)
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			return nil
		}
		for _, c := range batch {
			if err := visit(c); err != nil {
				return err
			}
			offset++
		}
	}
}
func (s *Store) readCaseBatch(id string, offset int) ([]CaseResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.expiredRun(id) {
		return nil, ErrDataExpired
	}
	rows, err := s.db.Query("SELECT body FROM run_cases WHERE run_id=? AND ordinal>=? ORDER BY ordinal LIMIT ?", id, offset, CaseBatchSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	batch := []CaseResult{}
	size := 0
	for rows.Next() {
		var body []byte
		if err := rows.Scan(&body); err != nil {
			return nil, err
		}
		if len(batch) > 0 && size+len(body) > MaxMetadataBytes {
			break
		}
		var c CaseResult
		if err := decodeJSON(body, &c); err != nil {
			return nil, err
		}
		batch = append(batch, c)
		size += len(body)
	}
	return batch, rows.Err()
}

type CaseSummary struct {
	ID              string `json:"id"`
	Ordinal         int    `json:"ordinal"`
	Status          string `json:"status"`
	Reason          string `json:"reason,omitempty"`
	ResponsePreview string `json:"response_preview"`
	Observed        int    `json:"observed"`
	Anomaly         bool   `json:"anomaly"`
	Episode         string `json:"episode,omitempty"`
	Candidate       string `json:"candidate,omitempty"`
	OriginalID      string `json:"original_id,omitempty"`
}
type CasePage struct {
	Items  []CaseSummary `json:"items"`
	Total  int           `json:"total"`
	Offset int           `json:"offset"`
	Limit  int           `json:"limit"`
}
type CaseFilter struct {
	Episode, Candidate, OriginalID string
	Anomalies                      bool
}

func (s *Store) Cases(id string, offset, limit int, filter CaseFilter) (CasePage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := CasePage{Items: []CaseSummary{}, Offset: offset, Limit: limit}
	if offset < 0 || limit < 1 || limit > 100 {
		return out, errors.New("invalid page")
	}
	if s.expiredRun(id) {
		return out, ErrDataExpired
	}
	where := "run_id=?"
	args := []any{id}
	for _, v := range []struct{ k, v string }{{"episode", filter.Episode}, {"candidate", filter.Candidate}, {"original_id", filter.OriginalID}} {
		if v.v != "" {
			where += " AND " + v.k + "=?"
			args = append(args, v.v)
		}
	}
	if filter.Anomalies {
		where += " AND anomaly=1"
	}
	if err := s.db.QueryRow("SELECT count(*) FROM run_cases WHERE "+where, args...).Scan(&out.Total); err != nil {
		return out, err
	}
	args = append(args, limit, offset)
	rows, err := s.db.Query("SELECT ordinal,body FROM run_cases WHERE "+where+" ORDER BY ordinal LIMIT ? OFFSET ?", args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var ordinal int
		var body []byte
		if err := rows.Scan(&ordinal, &body); err != nil {
			return out, err
		}
		var c CaseResult
		if err := decodeJSON(body, &c); err != nil {
			return out, err
		}
		preview, _ := json.Marshal(c.Result.Response)
		text := []rune(string(preview))
		if len(text) > 240 {
			text = append(text[:240], '…')
		}
		item := CaseSummary{ID: c.Case.ID, Ordinal: ordinal, Status: c.Status, Reason: c.Reason, ResponsePreview: string(text), Anomaly: caseAnomaly(c)}
		for _, m := range c.Metrics {
			if m.Status == "observed" {
				item.Observed++
			}
		}
		item.Episode, _ = c.Case.Metadata["episode_id"].(string)
		item.Candidate, _ = c.Case.Metadata["seven_eval_candidate"].(string)
		item.OriginalID, _ = c.Case.Metadata["seven_eval_case_id"].(string)
		out.Items = append(out.Items, item)
	}
	return out, rows.Err()
}
func (s *Store) Case(id, caseID string) (CaseResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var c CaseResult
	if s.expiredRun(id) {
		return c, ErrDataExpired
	}
	var body []byte
	if err := s.db.QueryRow("SELECT body FROM run_cases WHERE run_id=? AND case_id=?", id, caseID).Scan(&body); err != nil {
		return c, err
	}
	return c, decodeJSON(body, &c)
}

// ImportRun consumes an already validated portable stream without starting a worker.
func (s *Store) ImportRun(input Run) (Run, error) {
	s.importMu.Lock()
	defer s.importMu.Unlock()
	if _, err := s.Get(input.ID); err == nil {
		return Run{}, errors.New("run already exists")
	}
	r := runMetadata(input)
	r.Completed = 0
	r.Status = "running"
	if err := s.Commit(&r, "run.started", nil); err != nil {
		return r, err
	}
	finished := false
	defer func() {
		if !finished {
			r.Status = "partial"
			r.Message = "Import interrupted"
			_ = s.Commit(&r, "run.failed", nil)
		}
	}()
	inputDir := filepath.Join(s.Root, "runs", r.ID, "inputs")
	if err := os.MkdirAll(inputDir, 0700); err != nil {
		return r, err
	}
	f, err := os.CreateTemp(inputDir, ".import-")
	if err != nil {
		return r, err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	encoder := json.NewEncoder(f)
	if err = input.ForEachInput(func(c Case) error { return encoder.Encode(c) }); err != nil {
		return r, err
	}
	if err = f.Sync(); err != nil {
		return r, err
	}
	if err = f.Close(); err != nil {
		return r, err
	}
	if err = os.Rename(f.Name(), filepath.Join(inputDir, "cases.jsonl")); err != nil {
		return r, err
	}
	if err = syncDirectory(inputDir); err != nil {
		return r, err
	}
	err = input.ForEachCase(func(c CaseResult) error { r.Completed++; return s.Commit(&r, "case.completed", c) })
	if err != nil {
		return r, err
	}
	r.Status = input.Status
	r.Message = input.Message
	if r.Completed != input.Completed {
		return r, errors.New("import count mismatch")
	}
	if err := s.Commit(&r, "run.imported", nil); err != nil {
		return r, err
	}
	finished = true
	return s.Get(r.ID)
}

func validateStateVersion(db *sql.DB) error {
	rows, err := db.Query("PRAGMA table_info(run_cases)")
	if err != nil {
		return err
	}
	defer rows.Close()
	found := false
	for rows.Next() {
		var cid, notnull, pk int
		var name, kind string
		var def any
		if err := rows.Scan(&cid, &name, &kind, &notnull, &def, &pk); err != nil {
			return err
		}
		if name == "case_id" {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("unsupported state format; remove generated state and rerun (requires %s)", Version)
	}
	return rows.Err()
}

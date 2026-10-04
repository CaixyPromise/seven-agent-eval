package core

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

type Store struct {
	Root           string
	db             *sql.DB
	lock           *os.File
	mu             sync.Mutex
	sequence       map[string]int
	journalFailure error
	appendRecord   func(string, []byte) error
	caseBytes      map[string]int
}

// ProjectionError means the event is already durable. Never append stale query
// state in response; the next open rebuilds the projections from the journal.
type ProjectionError struct{ Err error }

func (e *ProjectionError) Error() string {
	return "event committed; projection requires recovery: " + e.Err.Error()
}
func (e *ProjectionError) Unwrap() error { return e.Err }

func NewStore(root string) (*Store, error) {
	if e := os.MkdirAll(filepath.Join(root, "runs"), 0700); e != nil {
		return nil, e
	}
	f, e := os.OpenFile(filepath.Join(root, "owner.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		f.Close()
		return nil, errors.New("workspace in use; stop its server or use the browser")
	}
	db, e := sql.Open("sqlite", filepath.Join(root, "index.sqlite"))
	if e != nil {
		f.Close()
		return nil, e
	}
	db.SetMaxOpenConns(1)
	s := &Store{Root: root, db: db, lock: f, sequence: map[string]int{}, caseBytes: map[string]int{}}
	if _, e = db.Exec("CREATE TABLE IF NOT EXISTS runs (id TEXT PRIMARY KEY, body BLOB NOT NULL)"); e != nil {
		s.Close()
		return nil, e
	}
	if _, e = db.Exec("CREATE TABLE IF NOT EXISTS run_cases (run_id TEXT NOT NULL, ordinal INTEGER NOT NULL, body BLOB NOT NULL, PRIMARY KEY(run_id,ordinal))"); e != nil {
		s.Close()
		return nil, e
	}
	if e = s.rebuild(); e != nil {
		s.Close()
		return nil, e
	}
	return s, nil
}
func (s *Store) Close() error {
	e := s.db.Close()
	syscall.Flock(int(s.lock.Fd()), syscall.LOCK_UN)
	s.lock.Close()
	return e
}
func (s *Store) Commit(r *Run, eventType string, data any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.commit(r, eventType, data)
}
func (s *Store) commit(r *Run, eventType string, data any) error {
	if s.expiredRun(r.ID) {
		return ErrDataExpired
	}
	if s.journalFailure != nil {
		return errors.New("journal unavailable; reopen the store to recover")
	}
	if !validID.MatchString(r.ID) {
		return errors.New("invalid run id")
	}
	dir := filepath.Join(s.Root, "runs", r.ID)
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	ev := Event{Sequence: s.sequence[r.ID] + 1, Type: eventType, Time: time.Now().UTC(), Data: data}
	projection := *r
	var delta *CaseResult
	if eventType == "case.completed" {
		c, ok := data.(CaseResult)
		if !ok || len(r.Cases) == 0 || r.Completed != len(r.Cases) {
			return errors.New("invalid case projection")
		}
		delta = &c
		projection.Cases = nil
	}
	body, e := json.Marshal(&projection)
	if e != nil {
		return e
	}
	if len(body) > 64<<20 {
		return errors.New("run exceeds 64 MiB")
	}
	pretty, e := json.MarshalIndent(&projection, "", "  ")
	if e != nil {
		return e
	}
	if len(pretty) > 64<<20 {
		return errors.New("result exceeds 64 MiB")
	}
	if delta != nil {
		b, err := json.MarshalIndent(delta, "    ", "  ")
		if err != nil {
			return err
		}
		if s.caseBytes[r.ID]+len(b)+len(pretty)+16 > 64<<20 {
			return errors.New("result exceeds 64 MiB")
		}
	}
	if eventType != "case.completed" {
		ev.Run = body
	}
	line, e := json.Marshal(ev)
	if e != nil {
		return e
	}
	if len(line)+1 > 64<<20 {
		return errors.New("journal record exceeds 64 MiB")
	}
	write := s.appendRecord
	if write == nil {
		write = appendJournal
	}
	if e = write(filepath.Join(dir, "events.jsonl"), append(line, '\n')); e != nil {
		s.journalFailure = e
		return e
	}
	s.sequence[r.ID] = ev.Sequence
	// The journal is durable first; both following stores can be reconstructed.
	// During execution this file is a lightweight checkpoint. Get assembles
	// complete cases from the SQLite projection; terminal checkpoints are full.
	if e = atomicJSON(filepath.Join(dir, "result.json"), &projection); e != nil {
		return &ProjectionError{Err: e}
	}
	e = s.project(r, delta)
	if e != nil {
		return &ProjectionError{Err: e}
	}
	return e
}
func (s *Store) Get(id string) (Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var r Run
	if s.expiredRun(id) {
		return r, ErrDataExpired
	}
	var b []byte
	if e := s.db.QueryRow("SELECT body FROM runs WHERE id=?", id).Scan(&b); e != nil {
		return r, e
	}
	e := decodeJSON(b, &r)
	if e != nil {
		return r, e
	}
	rows, e := s.db.Query("SELECT body FROM run_cases WHERE run_id=? ORDER BY ordinal", id)
	if e != nil {
		return r, e
	}
	defer rows.Close()
	for rows.Next() {
		var b []byte
		var c CaseResult
		if e = rows.Scan(&b); e != nil {
			return r, e
		}
		if e = decodeJSON(b, &c); e != nil {
			return r, e
		}
		r.Cases = append(r.Cases, c)
	}
	return r, rows.Err()
}

// Separate case rows prevent rewriting all prior evidence on every completion.
// The projection is transactional and entirely rebuildable from the journal.
func (s *Store) project(r *Run, delta *CaseResult) error {
	meta := *r
	meta.Cases = nil
	if r.Cases != nil {
		meta.Cases = []CaseResult{}
	}
	body, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("INSERT INTO runs(id,body) VALUES(?,?) ON CONFLICT(id) DO UPDATE SET body=excluded.body", r.ID, body); err != nil {
		return err
	}
	bytes := s.caseBytes[r.ID]
	items := r.Cases
	start := 0
	if delta != nil {
		items = []CaseResult{*delta}
		start = len(r.Cases) - 1
	} else {
		if _, err = tx.Exec("DELETE FROM run_cases WHERE run_id=?", r.ID); err != nil {
			return err
		}
		bytes = 0
	}
	for i, c := range items {
		b, e := json.Marshal(c)
		if e != nil {
			return e
		}
		if _, err = tx.Exec("INSERT INTO run_cases(run_id,ordinal,body) VALUES(?,?,?)", r.ID, start+i, b); err != nil {
			return err
		}
		pretty, e := json.MarshalIndent(c, "    ", "  ")
		if e != nil {
			return e
		}
		bytes += len(pretty) + 16
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	s.caseBytes[r.ID] = bytes
	return nil
}

// List returns run metadata without loading every historical CaseResult.
// Get provides the full run when the user selects a specific result.
func (s *Store) List() ([]Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, e := s.db.Query("SELECT body FROM runs ORDER BY id DESC")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Run{}
	for rows.Next() {
		var b []byte
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		var r Run
		if e = decodeJSON(b, &r); e != nil {
			return nil, e
		}
		if s.expiredRun(r.ID) {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (s *Store) rebuild() error {
	dirs, e := os.ReadDir(filepath.Join(s.Root, "runs"))
	if e != nil {
		return e
	}
	// Projection rows are recreated from authoritative journals, including after index deletion.
	if _, e = s.db.Exec("DELETE FROM runs"); e != nil {
		return e
	}
	if _, e = s.db.Exec("DELETE FROM run_cases"); e != nil {
		return e
	}
	for _, dir := range dirs {
		if !dir.IsDir() || !validID.MatchString(dir.Name()) {
			continue
		}
		if s.expiredRun(dir.Name()) {
			continue
		}
		p := filepath.Join(s.Root, "runs", dir.Name(), "events.jsonl")
		f, e := os.Open(p)
		if e != nil {
			if os.IsNotExist(e) {
				continue
			} // A failed preparation may leave only an uncommitted source snapshot.
			return e
		}
		reader := bufio.NewReader(f)
		var r Run
		var offset int64
		seq := 0
		for {
			line, err := readJournalLine(reader, 64<<20)
			if err == io.EOF {
				break
			}
			if err != nil {
				f.Close()
				return err
			}
			if len(line) > 64<<20 {
				f.Close()
				return errors.New("journal record too large")
			}
			var ev Event
			if e = decodeJSON(line, &ev); e != nil {
				f.Close()
				return fmt.Errorf("corrupt journal %s", dir.Name())
			}
			if ev.Sequence != seq+1 {
				f.Close()
				return errors.New("journal sequence gap")
			}
			seq = ev.Sequence
			if ev.Run != nil {
				if e = decodeJSON(ev.Run, &r); e != nil {
					f.Close()
					return e
				}
			} else if ev.Type == "case.completed" {
				b, _ := json.Marshal(ev.Data)
				var cr CaseResult
				if e = decodeJSON(b, &cr); e != nil {
					f.Close()
					return e
				}
				r.Cases = append(r.Cases, cr)
				r.Completed = len(r.Cases)
			}
			offset += int64(len(line))
		}
		f.Close()
		stat, err := os.Stat(p)
		if err != nil {
			return err
		}
		if stat.Size() != offset {
			if e = truncateJournal(p, offset); e != nil {
				return e
			}
		}
		if r.ID == "" {
			continue
		}
		s.sequence[r.ID] = seq
		if r.Status == "running" || r.Status == "processing" {
			r.Status = "partial"
			r.Message = "Interrupted run recovered; uncertain operations were not replayed."
			if e = s.commit(&r, "run.recovered", nil); e != nil {
				return e
			}
		} else {
			if e = s.project(&r, nil); e != nil {
				return e
			}
			if e = atomicJSON(filepath.Join(s.Root, "runs", r.ID, "result.json"), &r); e != nil {
				return e
			}
		}
	}
	return nil
}

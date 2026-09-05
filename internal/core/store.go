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
	Root     string
	db       *sql.DB
	lock     *os.File
	mu       sync.Mutex
	sequence map[string]int
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
	s := &Store{Root: root, db: db, lock: f, sequence: map[string]int{}}
	if _, e = db.Exec("CREATE TABLE IF NOT EXISTS runs (id TEXT PRIMARY KEY, body BLOB NOT NULL)"); e != nil {
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
	if !validID.MatchString(r.ID) {
		return errors.New("invalid run id")
	}
	dir := filepath.Join(s.Root, "runs", r.ID)
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	ev := Event{Sequence: s.sequence[r.ID] + 1, Type: eventType, Time: time.Now().UTC(), Data: data}
	body, e := json.Marshal(r)
	if e != nil {
		return e
	}
	if len(body) > 64<<20 {
		return errors.New("run exceeds 64 MiB")
	}
	pretty, e := json.MarshalIndent(r, "", "  ")
	if e != nil {
		return e
	}
	if len(pretty) > 64<<20 {
		return errors.New("result exceeds 64 MiB")
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
	f, e := os.OpenFile(filepath.Join(dir, "events.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if e != nil {
		return e
	}
	_, e = f.Write(append(line, '\n'))
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	s.sequence[r.ID] = ev.Sequence
	// The journal is durable first; both following stores can be reconstructed.
	if e = atomicJSON(filepath.Join(dir, "result.json"), r); e != nil {
		return &ProjectionError{Err: e}
	}
	_, e = s.db.Exec("INSERT INTO runs(id,body) VALUES(?,?) ON CONFLICT(id) DO UPDATE SET body=excluded.body", r.ID, body)
	if e != nil {
		return &ProjectionError{Err: e}
	}
	return e
}
func (s *Store) Get(id string) (Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var r Run
	var b []byte
	if e := s.db.QueryRow("SELECT body FROM runs WHERE id=?", id).Scan(&b); e != nil {
		return r, e
	}
	e := decodeJSON(b, &r)
	return r, e
}
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
	for _, dir := range dirs {
		if !dir.IsDir() || !validID.MatchString(dir.Name()) {
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
			line, err := reader.ReadBytes('\n')
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
		if e = os.Truncate(p, offset); e != nil {
			return e
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
			body, _ := json.Marshal(r)
			if _, e = s.db.Exec("INSERT INTO runs(id,body) VALUES(?,?)", r.ID, body); e != nil {
				return e
			}
			if e = atomicJSON(filepath.Join(s.Root, "runs", r.ID, "result.json"), &r); e != nil {
				return e
			}
		}
	}
	return nil
}

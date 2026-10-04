package core

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestStoreFailedAppendRequiresReopen(t *testing.T) {
	root := t.TempDir()
	s, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r := Run{ID: "failure", Status: "running"}
	p := filepath.Join(root, "runs", r.ID, "events.jsonl")
	if err = os.MkdirAll(p, 0700); err != nil {
		t.Fatal(err)
	}
	if err = s.Commit(&r, "run.started", nil); err == nil {
		t.Fatal("append incorrectly acknowledged")
	}
	if err = os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if err = s.Commit(&r, "run.started", nil); err == nil {
		t.Fatal("failed journal accepted another write without recovery")
	}
}

func TestJournalReaderBoundedTail(t *testing.T) {
	for _, body := range []string{strings.Repeat("x", 33), strings.Repeat("x", 33) + "\n"} {
		if _, err := readJournalLine(bufio.NewReaderSize(strings.NewReader(body), 16), 32); err == nil || err == io.EOF {
			t.Fatal("oversized tail accepted", err)
		}
	}
	b, err := readJournalLine(bufio.NewReader(strings.NewReader("partial")), 32)
	if err != io.EOF || string(b) != "partial" {
		t.Fatal("ordinary interrupted tail changed")
	}
}

// Inject a short disk write, not an actual full host filesystem. The partial
// bytes exist on disk and must be removed on replay before retrying the event.
func failPartialAppend(path string, line []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(line[:len(line)/2])
	if err == nil {
		err = f.Sync()
	}
	ce := f.Close()
	if err != nil {
		return err
	}
	if ce != nil {
		return ce
	}
	return syscall.ENOSPC
}

func TestDiskFailureDoesNotACKAndRestarts(t *testing.T) {
	e, m, _ := newStoppedLive(t)
	event := liveEvent("disk", 1, "hello")
	m.appendRecord = failPartialAppend
	got, err := m.Ingest(context.Background(), IngestBatch{PlanID: "production", Events: []ProductionEvent{event}})
	if !errors.Is(err, syscall.ENOSPC) || got.Accepted != 0 || len(m.records) != 0 {
		t.Fatalf("false ACK: %+v %v", got, err)
	}
	m.appendRecord = nil
	if _, err = m.Ingest(context.Background(), IngestBatch{PlanID: "production", Events: []ProductionEvent{event}}); err == nil {
		t.Fatal("uncertain journal reused")
	}
	m.Close()
	next, err := NewLiveManager(e)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	got, err = next.Ingest(context.Background(), IngestBatch{PlanID: "production", Events: []ProductionEvent{event}})
	if err != nil || got.Accepted != 1 {
		t.Fatalf("recovery: %+v %v", got, err)
	}
}

func TestOfflineDiskFailurePreservesPriorCases(t *testing.T) {
	root := t.TempDir()
	s, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	r := Run{ID: "disk", Status: "running"}
	if err = s.Commit(&r, "run.started", nil); err != nil {
		t.Fatal(err)
	}
	c := CaseResult{Case: Case{ID: "confirmed"}, Status: "completed"}
	r.Cases = append(r.Cases, c)
	r.Completed = 1
	if err = s.Commit(&r, "case.completed", c); err != nil {
		t.Fatal(err)
	}
	s.appendRecord = failPartialAppend
	if err = s.Commit(&r, "run.finished", nil); !errors.Is(err, syscall.ENOSPC) {
		t.Fatal(err)
	}
	s.Close()
	s, err = NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, err := s.Get(r.ID)
	if err != nil || got.Status != "partial" || len(got.Cases) != 1 {
		t.Fatalf("lost confirmed data: %+v %v", got, err)
	}
}

func TestStoreRebuildDoesNotRefreshRetentionAge(t *testing.T) {
	root := t.TempDir()
	s, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	r := Run{ID: "age", Status: "completed"}
	if err = s.Commit(&r, "run.finished", nil); err != nil {
		t.Fatal(err)
	}
	s.Close()
	p := filepath.Join(root, "runs", r.ID, "events.jsonl")
	before := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err = os.Chtimes(p, before, before); err != nil {
		t.Fatal(err)
	}
	s, err = NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	st, err := os.Stat(p)
	if err != nil || !st.ModTime().Equal(before) {
		t.Fatal("rebuild refreshed authoritative age", st, err)
	}
}

func TestOfflineSQLProjectionRebuildsIncrementalCase(t *testing.T) {
	root := t.TempDir()
	s, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	r := Run{ID: "sql", Status: "running"}
	if err = s.Commit(&r, "run.started", nil); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec("DROP TABLE run_cases"); err != nil {
		t.Fatal(err)
	}
	c := CaseResult{Case: Case{ID: "durable"}, Status: "completed"}
	r.Cases = []CaseResult{c}
	r.Completed = 1
	var projection *ProjectionError
	if err = s.Commit(&r, "case.completed", c); !errors.As(err, &projection) {
		t.Fatal("missing committed distinction", err)
	}
	s.Close()
	s, err = NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, err := s.Get(r.ID)
	if err != nil || len(got.Cases) != 1 || got.Cases[0].Case.ID != "durable" || got.Status != "partial" {
		t.Fatal(got, err)
	}
}

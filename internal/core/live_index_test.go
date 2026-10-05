package core

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func indexedFixture(t *testing.T) (*Engine, *LiveManager) {
	t.Helper()
	e, _, root := fixture(t)
	put(t, root, "plans/production.yaml", "schema_version: evaluate.plan/v1\nid: production\nmode: stream\nstream:\n  unit: trace\n")
	m, err := NewLiveManager(e)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	return e, m
}

func TestLiveProjectionFailureACKAndAtomicRecovery(t *testing.T) {
	_, m := indexedFixture(t)
	_, err := m.db.Exec("CREATE TRIGGER reject_projection BEFORE INSERT ON live_records BEGIN SELECT RAISE(FAIL,'controlled projection failure'); END")
	if err != nil {
		t.Fatal(err)
	}
	e := liveEvent("durable", 1, "hello")
	ack, err := m.Ingest(context.Background(), IngestBatch{PlanID: "production", Events: []ProductionEvent{e}})
	if err != nil || ack.Accepted != 1 || !m.projectionDirty {
		t.Fatalf("journal ACK: %+v %v", ack, err)
	}
	var n int
	if err = m.db.QueryRow("SELECT count(*) FROM live_projection").Scan(&n); err != nil || n != 0 {
		t.Fatalf("partial transaction: %d %v", n, err)
	}
	if _, err = m.db.Exec("DROP TRIGGER reject_projection"); err != nil {
		t.Fatal(err)
	}
	ack, err = m.Ingest(context.Background(), IngestBatch{PlanID: "production", Events: []ProductionEvent{e}})
	if err != nil || ack.Duplicate != 1 {
		t.Fatalf("recovered receipt: %+v %v", ack, err)
	}
	m.mu.Lock()
	n, err = m.pendingLocked("production")
	m.mu.Unlock()
	if err != nil || n != 1 {
		t.Fatalf("recovered pending: %d %v", n, err)
	}
}

func TestLiveCommitBusyACKAndRecovery(t *testing.T) {
	_, m := indexedFixture(t)
	reader, err := sql.Open("sqlite", filepath.Join(m.root, "projection.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	tx, err := reader.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var n int
	if err = tx.QueryRow("SELECT count(*) FROM live_records").Scan(&n); err != nil {
		t.Fatal(err)
	}
	e := liveEvent("commit-busy", 1, "hello")
	ack, err := m.Ingest(context.Background(), IngestBatch{PlanID: "production", Events: []ProductionEvent{e}})
	if err != nil || ack.Accepted != 1 || !m.projectionDirty {
		t.Fatalf("durable ACK despite commit lock: %+v %v", ack, err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	ack, err = m.Ingest(context.Background(), IngestBatch{PlanID: "production", Events: []ProductionEvent{e}})
	if err != nil || ack.Duplicate != 1 {
		t.Fatalf("repair after reader released: %+v %v", ack, err)
	}
	m.mu.Lock()
	n, err = m.pendingLocked("production")
	m.mu.Unlock()
	if err != nil || n != 1 {
		t.Fatalf("lost durable unit: %d %v", n, err)
	}
}

func TestRestoredOTLPEmissionPrecedesAssembly(t *testing.T) {
	e, m := indexedFixture(t)
	event := liveEvent("restored-otlp", 1, "hello")
	event.Case.Metadata = map[string]any{"seven_eval_otlp_assembly": "assembly-1"}
	r := &liveRecord{Unit: "trace:restored", Sequence: 1, PlanID: "production", Event: event, Digest: Digest(event), WindowTime: event.EventTime, WindowStart: event.EventTime.Truncate(time.Minute)}
	if err := m.appendLocked(liveJournal{Type: "event.restored", Record: r}); err != nil {
		t.Fatal(err)
	}
	a := &otlpAssembly{Key: "assembly-1", PlanID: "production", Revision: 1, FirstAt: event.EventTime, UpdatedAt: event.EventTime, DueAt: event.EventTime}
	if err := m.appendLocked(liveJournal{Type: "otlp.updated", Assembly: a}); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := NewLiveManager(e)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	var emitted int
	if err = next.db.QueryRow("SELECT emitted FROM live_assemblies WHERE key=?", a.Key).Scan(&emitted); err != nil || emitted != 1 {
		t.Fatalf("compacted receipt resurrected as pending: %d %v", emitted, err)
	}
}

func TestLiveFrozenRangeDoesNotMixRevisionOrScoring(t *testing.T) {
	_, m := indexedFixture(t)
	ctx := context.Background()
	e := liveEvent("original", 1, "hello")
	e.EventTime = time.Now().Add(-5 * time.Minute)
	if ack, err := m.Ingest(ctx, IngestBatch{PlanID: "production", Events: []ProductionEvent{e}}); err != nil || ack.Accepted != 1 {
		t.Fatal(ack, err)
	}
	base, err := m.plan(ctx, "production")
	if err != nil {
		t.Fatal(err)
	}
	start := e.EventTime.Truncate(time.Minute)
	m.mu.Lock()
	err = m.snapshotRangeLocked("fixed", "production", start, start.Add(time.Minute), false)
	m.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	m.scorePending(ctx)
	revision := e
	revision.EventID = "revision"
	revision.EvidenceRevision = 2
	revision.Result.Response = ""
	if ack, err := m.Ingest(ctx, IngestBatch{PlanID: "production", Events: []ProductionEvent{revision}}); err != nil || ack.Accepted != 1 {
		t.Fatal(ack, err)
	}
	m.scorePending(ctx)
	w, err := m.windowIndexed(ctx, base, start, "fixed")
	if err != nil || w.Scope.Received != 1 || w.Scope.Pending != 1 || w.Completed != 0 {
		t.Fatalf("mixed snapshot: %+v %v", w.Scope, err)
	}
	report, err := m.Report(ctx, "production", start, start.Add(time.Minute))
	if err != nil || report.Current.Scope.Pending != 0 || report.Current.Scope.Received != 1 || report.Current.Completed != 1 {
		t.Fatalf("latest: %+v %v", report.Current, err)
	}
	if report.Current.Cases != nil || len(report.Current.Scope.Facts) != 0 {
		t.Fatal("report exposed unpaged evidence")
	}
}

func TestLiveQueriesUseIndexedRangesAndBudgets(t *testing.T) {
	_, m := indexedFixture(t)
	queries := []struct {
		query, index string
		args         []any
	}{
		{"SELECT count(*) FROM live_records WHERE plan=? AND expired=0 AND window_ns>=? AND window_ns<?", "live_event_range", []any{"production", 0, 1}},
		{"SELECT body FROM live_records WHERE scored=0 AND expired=0 AND seq>? ORDER BY seq LIMIT ?", "live_pending", []any{0, 32}},
		{"SELECT count(*) FROM live_decisions WHERE plan=? AND start_ns=? AND identity=? AND decision IN ('selected','interrupted')", "live_budget", []any{"production", 0, "judge"}},
		{"SELECT key FROM live_assemblies WHERE emitted=0 AND due_ns<=? ORDER BY due_ns,key LIMIT ?", "live_assembly_pending", []any{0, 32}},
	}
	for _, q := range queries {
		rows, err := m.db.Query("EXPLAIN QUERY PLAN "+q.query, q.args...)
		if err != nil {
			t.Fatal(err)
		}
		var plans []string
		for rows.Next() {
			var a, b, c int
			var detail string
			if err = rows.Scan(&a, &b, &c, &detail); err != nil {
				t.Fatal(err)
			}
			plans = append(plans, detail)
		}
		err = rows.Err()
		rows.Close()
		if err != nil || !strings.Contains(strings.Join(plans, " "), q.index) {
			t.Fatalf("missing index %s: %v %v", q.index, plans, err)
		}
	}
}

func TestLiveRangeLimitRejectsWithoutTruncation(t *testing.T) {
	_, m := indexedFixture(t)
	now := time.Now().Add(-5 * time.Minute)
	// Populate a disposable projection in one transaction; production capacity is
	// separately exercised via the actual HTTP ACK path.
	tx, err := m.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i <= MaxLiveRangeRecords; i++ {
		_, err = tx.Exec("INSERT INTO live_records VALUES(?,?,?,?,?,?,?,?,?,?,?)", i, "production", i, 1, i+1, now.UnixNano(), now.Truncate(time.Minute).UnixNano(), 0, 0, "{}", 0)
		if err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	_, err = m.Report(context.Background(), "production", now.Add(-time.Minute), now.Add(time.Minute))
	if !errors.Is(err, ErrLiveCapacity) {
		t.Fatalf("expected explicit capacity rejection: %v", err)
	}
}

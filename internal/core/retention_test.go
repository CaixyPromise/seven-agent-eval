package core

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func maintenance(t *testing.T, e *Engine, m *LiveManager) *LiveManager {
	t.Helper()
	m.Close()
	next, err := NewLiveManager(e)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { next.Close() })
	return next
}

func TestRetentionReceiptsExportAndRecovery(t *testing.T) {
	ctx := context.Background()
	e, m, _ := newStoppedLive(t)
	event := liveEvent("retained", 1, "hello")
	event.EventTime = time.Now().Add(-time.Hour)
	if _, err := m.Ingest(ctx, IngestBatch{PlanID: "production", Events: []ProductionEvent{event}}); err != nil {
		t.Fatal(err)
	}
	m.scorePending(ctx)
	report, err := m.Report(ctx, "production", time.Now().Add(-2*time.Hour), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	window := *report.Current
	bundle := filepath.Join(t.TempDir(), "window.seven.eval")
	if err = Export(window, bundle); err != nil {
		t.Fatal(err)
	}
	m = maintenance(t, e, m)
	p, err := m.PreviewRetention(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Records) != 1 || len(p.Windows) != 1 {
		t.Fatalf("preview: %+v", p)
	}
	if m.records[liveKey("production", event.Source, event.EventID)].Expired {
		t.Fatal("preview deleted evidence")
	}
	if err = m.SaveRetentionPreview(p); err != nil {
		t.Fatal(err)
	}
	m.compactPublish = func(string, string) error { return syscall.ENOSPC }
	if _, err = m.ApplyRetention(ctx, p.Digest); !errors.Is(err, syscall.ENOSPC) {
		t.Fatal("failed compaction acknowledged", err)
	}
	if _, err = m.Window(ctx, window.ID); err != nil {
		t.Fatal("failed compaction expired the old snapshot", err)
	}
	m.compactPublish = nil
	if _, err = m.ApplyRetention(ctx, p.Digest); err != nil {
		t.Fatal(err)
	}
	var rows int
	if err = m.db.QueryRow("SELECT COUNT(*) FROM live_projection").Scan(&rows); err != nil || rows == 0 {
		t.Fatal("compacted projection missing", rows, err)
	}
	m = maintenance(t, e, m)
	if _, err = m.Window(ctx, window.ID); !errors.Is(err, ErrDataExpired) {
		t.Fatal("expired query", err)
	}
	ack, err := m.Ingest(ctx, IngestBatch{PlanID: "production", Events: []ProductionEvent{event}})
	if err != nil || ack.Duplicate != 1 {
		t.Fatalf("receipt lost %+v %v", ack, err)
	}
	event.EventID = "revision-2"
	event.EvidenceRevision = 2
	event.EventTime = time.Now()
	ack, err = m.Ingest(ctx, IngestBatch{PlanID: "production", Events: []ProductionEvent{event}})
	if err != nil || ack.Rejected != 1 || ack.Items[0].Code != "DATA_EXPIRED" {
		t.Fatalf("expired unit replayed %+v %v", ack, err)
	}
	event.EventID = "unseen-old"
	event.TraceID = "unseen"
	event.EventTime = window.Scope.Start.Add(time.Second)
	ack, err = m.Ingest(ctx, IngestBatch{PlanID: "production", Events: []ProductionEvent{event}})
	if err != nil || ack.Rejected != 1 || ack.Items[0].Code != "DATA_EXPIRED" {
		t.Fatal("expired range reopened", ack, err)
	}
	m.scorePending(ctx)
	for _, r := range m.records {
		if !r.Expired || r.Case != nil || r.Event.Result.Response != nil {
			t.Fatal("payload retained or re-executed")
		}
	}
	imported, err := importFixture(t, bundle)
	if err != nil || Digest(imported) != Digest(window) {
		t.Fatal("portable archive changed", err)
	}
}

func TestRetentionPendingRevisionAndStalePreview(t *testing.T) {
	ctx := context.Background()
	e, m, _ := newStoppedLive(t)
	a := liveEvent("old", 1, "hello")
	a.EventTime = time.Now().Add(-time.Hour)
	if _, err := m.Ingest(ctx, IngestBatch{PlanID: "production", Events: []ProductionEvent{a}}); err != nil {
		t.Fatal(err)
	}
	m.scorePending(ctx)
	if _, err := m.Report(ctx, "production", time.Now().Add(-2*time.Hour), time.Now()); err != nil {
		t.Fatal(err)
	}
	a.EventID = "new-revision"
	a.EvidenceRevision = 2
	if _, err := m.Ingest(ctx, IngestBatch{PlanID: "production", Events: []ProductionEvent{a}}); err != nil {
		t.Fatal(err)
	}
	m = maintenance(t, e, m)
	p, err := m.PreviewRetention(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Records) != 0 || len(p.Windows) != 0 {
		t.Fatal("pending revision deleted", p)
	}
	if err = m.SaveRetentionPreview(p); err != nil {
		t.Fatal(err)
	}
	// Simulate a changed authoritative file between the two CLI invocations.
	f, err := os.OpenFile(filepath.Join(m.root, "events.jsonl"), os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(" ")
	f.Close()
	if _, err = m.ApplyRetention(ctx, p.Digest); err == nil {
		t.Fatal("stale preview applied")
	}
}

func TestRetentionOfflineReferenceProtection(t *testing.T) {
	e, m, _ := newStoppedLive(t)
	s := e.Store
	old := Run{ID: "old", Status: "completed", CreatedAt: time.Now().Add(-time.Hour)}
	dependent := Run{ID: "keep", Status: "partial", Effective: Effective{NodeRuntime: &NodeRuntime{SnapshotID: old.ID}}}
	for _, r := range []*Run{&old, &dependent} {
		if err := s.Commit(r, "run.finished", nil); err != nil {
			t.Fatal(err)
		}
	}
	p, err := m.PreviewRetention(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Runs) != 0 {
		t.Fatal("referenced runtime selected", p)
	}
	independent := Run{ID: "delete", Status: "completed"}
	if err = s.Commit(&independent, "run.finished", nil); err != nil {
		t.Fatal(err)
	}
	p, err = m.PreviewRetention(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Runs) != 1 || p.Runs[0] != "delete" {
		t.Fatal(p)
	}
	if err = m.SaveRetentionPreview(p); err != nil {
		t.Fatal(err)
	}
	if _, err = m.ApplyRetention(context.Background(), p.Digest); err != nil {
		t.Fatal(err)
	}
	if _, err = getFixture(t, s, "delete"); !errors.Is(err, ErrDataExpired) {
		t.Fatal("missing expiration", err)
	}
	if _, err = getFixture(t, s, "old"); err != nil {
		t.Fatal("dependency removed", err)
	}
	if err = s.Commit(&independent, "run.started", nil); !errors.Is(err, ErrDataExpired) {
		t.Fatal("expired run resurrected", err)
	}
}

func TestRetentionCancelledBeforePublish(t *testing.T) {
	e, m, _ := newStoppedLive(t)
	p, err := m.PreviewRetention(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = m.SaveRetentionPreview(p); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = m.ApplyRetention(ctx, p.Digest); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(e.Store.Root, "live", "events.jsonl")); !os.IsNotExist(err) {
		t.Fatal("cancelled apply published journal")
	}
}

func TestRetentionOTLPReceipts(t *testing.T) {
	ctx := context.Background()
	e, m, _ := newStoppedLive(t)
	body, err := os.ReadFile(filepath.Join(e.Worker.Home, "protocol/examples/otlp-openinference.json"))
	if err != nil {
		t.Fatal(err)
	}
	var req otlpRequest
	if err = decodeJSON(body, &req); err != nil {
		t.Fatal(err)
	}
	// Normalize the fixture's plan and clocks without changing the mapping.
	for i := range req.ResourceSpans {
		for j := range req.ResourceSpans[i].Resource.Attributes {
			a := &req.ResourceSpans[i].Resource.Attributes[j]
			if a.Key == "seven.eval.plan.id" {
				a.Value.StringValue = "production"
			}
		}
		for j := range req.ResourceSpans[i].ScopeSpans {
			for k := range req.ResourceSpans[i].ScopeSpans[j].Spans {
				s := &req.ResourceSpans[i].ScopeSpans[j].Spans[k]
				s.StartTimeUnixNano = uint64(time.Now().Add(-time.Hour).UnixNano())
				s.EndTimeUnixNano = s.StartTimeUnixNano + 1000000
			}
		}
	}
	body, _ = json.Marshal(req)
	if _, err = m.IngestOTLP(ctx, body); err != nil {
		t.Fatal(err)
	}
	for key, a := range m.assemblies {
		a.UpdatedAt = time.Now().Add(-time.Minute)
		m.assemblies[key] = a
	}
	m.flushOTLP(ctx)
	m.scorePending(ctx)
	if _, err = m.Report(ctx, "production", time.Now().Add(-2*time.Hour), time.Now()); err != nil {
		t.Fatal(err)
	}
	m = maintenance(t, e, m)
	p, err := m.PreviewRetention(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Assemblies) != 1 {
		t.Fatal("assembly not selected", p)
	}
	if err = m.SaveRetentionPreview(p); err != nil {
		t.Fatal(err)
	}
	if _, err = m.ApplyRetention(ctx, p.Digest); err != nil {
		t.Fatal(err)
	}
	m = maintenance(t, e, m)
	got, err := m.IngestOTLP(ctx, body)
	if err != nil || got.Duplicate != 1 {
		t.Fatal("span receipts lost", got, err)
	}
	for _, a := range m.assemblies {
		if len(a.Spans) != 0 || len(a.ExpiredSpans) == 0 {
			t.Fatal("span payload retained")
		}
	}
}

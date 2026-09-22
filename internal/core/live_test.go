package core

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func streamFixture(t *testing.T) (*Engine, *LiveManager, string) {
	t.Helper()
	e, _, root := fixture(t)
	put(t, root, "plans/production.yaml", "schema_version: evaluate.plan/v1\nid: production\nmode: stream\nstream:\n  unit: trace\n")
	m, err := NewLiveManager(e)
	if err != nil {
		t.Fatal(err)
	}
	m.Start()
	t.Cleanup(func() { _ = m.Close() })
	return e, m, root
}

func TestMapOpenInferenceOTLPJSON(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
	body, err := os.ReadFile(filepath.Join(root, "protocol/examples/otlp-openinference.json"))
	if err != nil {
		t.Fatal(err)
	}
	batch, err := MapOTLPJSON(body)
	if err != nil {
		t.Fatal(err)
	}
	if batch.PlanID != "production" || len(batch.Events) != 1 {
		t.Fatalf("unexpected batch: %+v", batch)
	}
	event := batch.Events[0]
	if event.Source != "controlled-agent" || event.Project != "demo" || event.TraceID == "" || len(event.Result.Contexts) != 1 || event.Result.Response != "hello" {
		t.Fatalf("mapping lost evidence: %+v", event)
	}
}
func liveEvent(id string, revision int, answer any) ProductionEvent {
	latency := 12.0
	return ProductionEvent{SchemaVersion: "evaluate.event/v1", Source: "fixture", Project: "test", EventID: id, EventTime: time.Now().UTC(), TraceID: "trace-" + id, EvidenceRevision: revision, Case: Case{ID: "case-" + id, Inputs: map[string]any{"question": "hello"}, References: map[string]any{"exact": "hello"}}, Result: SubjectResult{Response: answer, LatencyMS: &latency}}
}
func TestLiveIngestDeduplicatesScoresAndExports(t *testing.T) {
	_, m, _ := streamFixture(t)
	event := liveEvent("one", 1, "hello")
	got, err := m.Ingest(context.Background(), IngestBatch{PlanID: "production", Events: []ProductionEvent{event}})
	if err != nil || got.Accepted != 1 {
		t.Fatalf("ingest: %+v %v", got, err)
	}
	got, err = m.Ingest(context.Background(), IngestBatch{PlanID: "production", Events: []ProductionEvent{event}})
	if err != nil || got.Duplicate != 1 {
		t.Fatalf("duplicate: %+v %v", got, err)
	}
	deadline := time.Now().Add(5 * time.Second)
	var report LiveReport
	for time.Now().Before(deadline) {
		report, err = m.Report(context.Background(), "production", time.Now().Add(-time.Hour), time.Now().Add(time.Minute))
		if err == nil && report.Current != nil && report.Current.Scope.Scored == 1 {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	if report.Current == nil || report.Current.Scope.Received != 1 || report.Current.Scope.Scored != 1 {
		t.Fatalf("unexpected report: %+v", report)
	}
	p := t.TempDir() + "/window.seven.eval"
	if err = Export(*report.Current, p); err != nil {
		t.Fatal(err)
	}
	if _, err = ImportFile(p); err != nil {
		t.Fatal(err)
	}
}
func TestLiveConflictPauseAndRebuild(t *testing.T) {
	e, m, _ := streamFixture(t)
	event := liveEvent("one", 1, "hello")
	if _, err := m.Ingest(context.Background(), IngestBatch{PlanID: "production", Events: []ProductionEvent{event}}); err != nil {
		t.Fatal(err)
	}
	changed := event
	changed.Result.Response = "different"
	got, err := m.Ingest(context.Background(), IngestBatch{PlanID: "production", Events: []ProductionEvent{changed}})
	if err != nil || got.Rejected != 1 || got.Items[0].Code != "EVENT_CONFLICT" {
		t.Fatalf("conflict: %+v %v", got, err)
	}
	if _, err = m.SetPaused(context.Background(), "production", true); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Ingest(context.Background(), IngestBatch{PlanID: "production", Events: []ProductionEvent{liveEvent("two", 1, "hello")}}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(80 * time.Millisecond)
	report, err := m.Report(context.Background(), "production", time.Now().Add(-time.Hour), time.Now().Add(time.Minute))
	if err != nil || report.Current == nil || report.Current.Scope.Pending == 0 {
		t.Fatalf("paused report: %+v %v", report, err)
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	recovered, err := NewLiveManager(e)
	if err != nil {
		t.Fatal(err)
	}
	recovered.Start()
	defer recovered.Close()
	if _, err = recovered.SetPaused(context.Background(), "production", false); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		report, _ = recovered.Report(context.Background(), "production", time.Now().Add(-time.Hour), time.Now().Add(time.Minute))
		if report.Current != nil && report.Current.Scope.Scored == 2 {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatalf("recovered work was not scored: %+v", report)
}

func TestLiveRevisionReplacesWindowContribution(t *testing.T) {
	_, m, _ := streamFixture(t)
	first := liveEvent("one", 1, "wrong")
	first.TraceID = "trace-revised"
	second := liveEvent("two", 2, "hello")
	second.TraceID = "trace-revised"
	if _, err := m.Ingest(context.Background(), IngestBatch{PlanID: "production", Events: []ProductionEvent{first, second}}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		report, err := m.Report(context.Background(), "production", time.Now().Add(-time.Hour), time.Now().Add(time.Minute))
		if err == nil && report.Current != nil && report.Current.Scope.Scored == 1 {
			if report.Current.Scope.Received != 1 || report.Current.Scope.Revision < 2 || len(report.Current.Cases) != 1 || report.Current.Cases[0].Result.Response != "hello" {
				t.Fatalf("revision did not replace prior contribution: %+v", report.Current)
			}
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("revision was not scored")
}

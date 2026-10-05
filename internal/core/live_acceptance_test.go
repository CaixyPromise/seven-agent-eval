package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newStoppedLive(t *testing.T) (*Engine, *LiveManager, string) {
	t.Helper()
	e, _, root := fixture(t)
	put(t, root, "plans/production.yaml", "schema_version: evaluate.plan/v1\nid: production\nmode: stream\nstream:\n  unit: trace\n")
	m, err := NewLiveManager(e)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	return e, m, root
}

func TestLiveProjectionFailureACKRecovery(t *testing.T) {
	e, m, _ := newStoppedLive(t)
	if _, err := m.db.Exec("DROP TABLE live_projection"); err != nil {
		t.Fatal(err)
	}
	event := liveEvent("durable", 1, "hello")
	got, err := m.Ingest(context.Background(), IngestBatch{PlanID: "production", Events: []ProductionEvent{event}})
	if err != nil || got.Accepted != 1 || !m.projectionDegraded {
		t.Fatalf("ACK: %+v %v", got, err)
	}
	got, err = m.Ingest(context.Background(), IngestBatch{PlanID: "production", Events: []ProductionEvent{event}})
	if err != nil || got.Duplicate != 1 {
		t.Fatalf("retry: %+v %v", got, err)
	}
	m.Close()
	recovered, err := NewLiveManager(e)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	if len(testLiveRecords(t, recovered)) != 1 {
		t.Fatal("ACK event lost")
	}
	recovered.scorePending(context.Background())
	for _, r := range testLiveRecords(t, recovered) {
		if r.Case == nil {
			t.Fatal("recovery did not score")
		}
	}
}

func TestLiveProcessorRecomputeAndDisabled(t *testing.T) {
	e, m, root := newStoppedLive(t)
	event := liveEvent("latency", 1, "hello")
	event.EventTime = time.Now().Add(-5 * time.Minute)
	if _, err := m.Ingest(context.Background(), IngestBatch{PlanID: "production", Events: []ProductionEvent{event}}); err != nil {
		t.Fatal(err)
	}
	m.scorePending(context.Background())
	report, err := liveReport(t, m, context.Background(), "production", time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	online := report.Current
	if online == nil || !online.Scope.StatisticsAvailable || online.Scope.P95LatencyMS == nil {
		t.Fatal("statistics unavailable")
	}
	replay := cloneLiveRun(*online)
	if err := e.ProcessExisting(context.Background(), &replay); err != nil {
		t.Fatal(err)
	}
	if Digest(online.Outputs) != Digest(replay.Outputs) || Digest(online.Conclusions) != Digest(replay.Conclusions) {
		t.Fatal("online/offline mismatch")
	}
	put(t, root, "plans/disabled.yaml", "schema_version: evaluate.plan/v1\nid: disabled\nmode: stream\nprocessors: []\nstream:\n  unit: trace\n")
	if _, err := m.Ingest(context.Background(), IngestBatch{PlanID: "disabled", Events: []ProductionEvent{event}}); err != nil {
		t.Fatal(err)
	}
	m.scorePending(context.Background())
	disabled, err := liveReport(t, m, context.Background(), "disabled", time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if disabled.Current.Scope.StatisticsAvailable || len(disabled.Series) != 0 {
		t.Fatal("disabled processor substituted statistics")
	}
}

func TestLiveJudgePolicyAndReservationRecovery(t *testing.T) {
	e, m, root := newStoppedLive(t)
	put(t, root, "evaluators/judge.py", "from seven_evaluate import evaluator\n@evaluator(id='judge', execution_class='judge', metrics=['output.present'])\ndef evaluate(ctx):\n return [{'id':ctx['case']['id']+':judge','case_id':ctx['case']['id'],'metric_id':'output.present','status':'observed','value':True,'evaluator':'judge@1'}]\n")
	put(t, root, "plans/production.yaml", "schema_version: evaluate.plan/v1\nid: production\nmode: stream\nevaluators: [evaluator:basic, evaluator:judge]\nstream:\n  unit: trace\n  judge_sample_rate: 1\n  judge_per_window_budget: 1\n")
	first, second := liveEvent("a", 1, "hello"), liveEvent("b", 1, "hello")
	second.EventTime = first.EventTime
	if _, err := m.Ingest(context.Background(), IngestBatch{PlanID: "production", Events: []ProductionEvent{first, second}}); err != nil {
		t.Fatal(err)
	}
	m.scorePending(context.Background())
	one := testLiveRecords(t, m)[liveKey("production", first.Source, first.EventID)]
	two := testLiveRecords(t, m)[liveKey("production", second.Source, second.EventID)]
	if one.Decisions["evaluator:judge"] != "selected" || two.Decisions["evaluator:judge"] != "budget_exhausted" {
		t.Fatalf("budget/order: %v %v", one.Decisions, two.Decisions)
	}
	found := false
	for _, metric := range two.Case.Metrics {
		if metric.Status == "budget_exhausted" {
			found = true
			if metric.Value != nil {
				t.Fatal("budget became zero")
			}
		}
	}
	if !found {
		t.Fatal("budget status missing")
	}
	m.Close()
	recovered, err := NewLiveManager(e)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	if testLiveRecords(t, recovered)[liveKey("production", first.Source, first.EventID)].Decisions["evaluator:judge"] != "selected" {
		t.Fatal("reservation lost")
	}
}

func TestOTLPCrossBatchQuietAndRecovery(t *testing.T) {
	e, m, _ := newStoppedLive(t)
	body, err := os.ReadFile(filepath.Join(e.Worker.Home, "protocol/examples/otlp-openinference.json"))
	if err != nil {
		t.Fatal(err)
	}
	var req otlpRequest
	if err := decodeJSON(body, &req); err != nil {
		t.Fatal(err)
	}
	root := req.ResourceSpans[0].ScopeSpans[0].Spans[0]
	child := req.ResourceSpans[0].ScopeSpans[0].Spans[1]
	req.ResourceSpans[0].ScopeSpans[0].Spans = []otlpSpan{child}
	body, _ = json.Marshal(req)
	if got, err := m.IngestOTLP(context.Background(), body); err != nil || got.Accepted != 1 {
		t.Fatalf("child: %+v %v", got, err)
	}
	m.flushOTLP(context.Background())
	if len(testLiveRecords(t, m)) != 0 {
		t.Fatal("incomplete trace scored early")
	}
	m.Close()
	m, err = NewLiveManager(e)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	req.ResourceSpans[0].ScopeSpans[0].Spans = []otlpSpan{root}
	body, _ = json.Marshal(req)
	if _, err = m.IngestOTLP(context.Background(), body); err != nil {
		t.Fatal(err)
	}
	m.flushOTLP(context.Background())
	if len(testLiveRecords(t, m)) != 0 {
		t.Fatal("quiet period skipped")
	}
	for _, a := range testLiveAssemblies(t, m) {
		a.UpdatedAt = time.Now().Add(-6 * time.Second)
		setTestAssembly(t, m, a)
	}
	m.flushOTLP(context.Background())
	m.scorePending(context.Background())
	if len(testLiveRecords(t, m)) != 1 {
		t.Fatal("assembly not emitted")
	}
	for _, r := range testLiveRecords(t, m) {
		if len(r.Event.Result.Contexts) != 1 || r.Case == nil {
			t.Fatal("retrieval evidence lost")
		}
	}
	if got, err := m.IngestOTLP(context.Background(), body); err != nil || got.Duplicate != 1 {
		t.Fatalf("duplicate: %+v %v", got, err)
	}
}

func TestLiveRevisionIdentityAndFrozenWindow(t *testing.T) {
	_, m, _ := newStoppedLive(t)
	ctx := context.Background()
	first := liveEvent("first", 1, "original")
	first.EventTime = time.Now().Add(-10 * time.Minute)
	second := liveEvent("second", 1, "another")
	second.EventTime, second.Case.ID = first.EventTime, first.Case.ID
	if got, err := m.Ingest(ctx, IngestBatch{PlanID: "production", Events: []ProductionEvent{first, second}}); err != nil || got.Accepted != 2 {
		t.Fatalf("%+v %v", got, err)
	}
	m.scorePending(ctx)
	before, err := liveReport(t, m, ctx, "production", time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if before.Current.Cases[0].Case.ID == before.Current.Cases[1].Case.ID {
		t.Fatal("case identity collision")
	}
	revision := first
	revision.EventID, revision.EvidenceRevision, revision.Result.Response = "late", 2, "revised"
	revision.EventTime = time.Now().Add(-2 * time.Minute)
	if got, err := m.Ingest(ctx, IngestBatch{PlanID: "production", Events: []ProductionEvent{revision}}); err != nil || got.Accepted != 1 {
		t.Fatalf("%+v %v", got, err)
	}
	m.scorePending(ctx)
	after, err := liveReport(t, m, ctx, "production", time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Windows) != 1 || after.Current.Total != 2 || after.Current.Scope.Revision <= before.Current.Scope.Revision {
		t.Fatal("revision moved window or denominator")
	}
	frozen, err := liveWindow(t, m, ctx, before.Current.ID)
	if err != nil || Digest(frozen) != Digest(*before.Current) {
		t.Fatalf("snapshot changed: %v", err)
	}
	if got, err := m.Ingest(ctx, IngestBatch{PlanID: "production", Events: []ProductionEvent{revision}}); err != nil || got.Duplicate != 1 {
		t.Fatalf("duplicate: %+v %v", got, err)
	}
}

func TestLiveInterruptedJudgeNeverReplayed(t *testing.T) {
	e, m, root := newStoppedLive(t)
	put(t, root, "evaluators/judge.py", "from seven_evaluate import evaluator\n@evaluator(id='judge', execution_class='judge', metrics=['output.present'])\ndef evaluate(ctx):\n raise RuntimeError('must not be replayed')\n")
	put(t, root, "plans/production.yaml", "schema_version: evaluate.plan/v1\nid: production\nmode: stream\nevaluators: [evaluator:basic, evaluator:judge]\nstream:\n  unit: trace\n  judge_sample_rate: 1\n  judge_per_window_budget: 1\n")
	ctx := context.Background()
	event := liveEvent("uncertain", 1, "hello")
	event.EventTime = time.Now().Add(-5 * time.Minute)
	if _, err := m.Ingest(ctx, IngestBatch{PlanID: "production", Events: []ProductionEvent{event}}); err != nil {
		t.Fatal(err)
	}
	r := testLiveRecords(t, m)[liveKey("production", event.Source, event.EventID)]
	now := time.Now().UTC()
	r.StartedAt = &now
	r.Decisions = map[string]string{"evaluator:judge": "selected"}
	if err := m.appendLocked(liveJournal{Type: "event.started", PlanID: "production", Record: r}); err != nil {
		t.Fatal(err)
	}
	m.Close()
	recovered, err := NewLiveManager(e)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	recovered.scorePending(ctx)
	r = testLiveRecords(t, recovered)[liveKey("production", event.Source, event.EventID)]
	if r.Case == nil || r.Case.Reason != "evaluation_interrupted" {
		t.Fatal("uncertain judge replayed or lost")
	}
	report, err := liveReport(t, recovered, ctx, "production", time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if report.Current.Status != "partial" || report.Current.Scope.Failed != 1 {
		t.Fatal("interruption counted as success")
	}
}

func TestLivePauseAndTruncatedUnacknowledgedTail(t *testing.T) {
	e, m, _ := newStoppedLive(t)
	ctx := context.Background()
	if _, err := m.SetPaused(ctx, "production", true); err != nil {
		t.Fatal(err)
	}
	event := liveEvent("paused", 1, "hello")
	if got, err := m.Ingest(ctx, IngestBatch{PlanID: "production", Events: []ProductionEvent{event}}); err != nil || got.Accepted != 1 {
		t.Fatalf("%+v %v", got, err)
	}
	m.scorePending(ctx)
	for _, r := range testLiveRecords(t, m) {
		if r.StartedAt != nil {
			t.Fatal("dispatch while paused")
		}
	}
	path := filepath.Join(m.root, "events.jsonl")
	m.Close()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.WriteString(`{"sequence":`); err != nil {
		t.Fatal(err)
	}
	f.Close()
	recovered, err := NewLiveManager(e)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	if !recovered.paused["production"] || len(testLiveRecords(t, recovered)) != 1 {
		t.Fatal("durable pause/ACK lost")
	}
	if _, err = recovered.SetPaused(ctx, "production", false); err != nil {
		t.Fatal(err)
	}
	recovered.scorePending(ctx)
	for _, r := range testLiveRecords(t, recovered) {
		if r.Case == nil {
			t.Fatal("resume failed")
		}
	}
}

func TestLiveJudgeRequiresPolicyAndZeroSample(t *testing.T) {
	_, m, root := newStoppedLive(t)
	ctx := context.Background()
	put(t, root, "evaluators/judge.py", "from seven_evaluate import evaluator\n@evaluator(id='judge', execution_class='judge', metrics=['output.present'])\ndef evaluate(ctx):\n raise RuntimeError('must not run')\n")
	put(t, root, "plans/missing.yaml", "schema_version: evaluate.plan/v1\nid: missing\nmode: stream\nevaluators: [evaluator:judge]\nstream:\n  unit: trace\n")
	if _, err := m.plan(ctx, "missing"); err == nil {
		t.Fatal("judge missing policy accepted")
	}
	put(t, root, "plans/zero.yaml", "schema_version: evaluate.plan/v1\nid: zero\nmode: stream\nevaluators: [evaluator:basic, evaluator:judge]\nstream:\n  unit: trace\n  judge_sample_rate: 0\n  judge_per_window_budget: 1\n")
	if _, err := m.Ingest(ctx, IngestBatch{PlanID: "zero", Events: []ProductionEvent{liveEvent("sample-zero", 1, "ok")}}); err != nil {
		t.Fatal(err)
	}
	m.scorePending(ctx)
	for _, r := range testLiveRecords(t, m) {
		if r.Decisions["evaluator:judge"] != "skipped" || r.Case == nil || r.Case.Status != "completed" {
			t.Fatal("zero sampling ran judge")
		}
		observed := false
		for _, metric := range r.Case.Metrics {
			if metric.Status == "observed" {
				observed = true
			}
		}
		if !observed {
			t.Fatal("sampling suppressed deterministic evaluator")
		}
	}
}

func TestLivePauseBoundaryAllowsOnlyInflightCase(t *testing.T) {
	_, m, root := newStoppedLive(t)
	ctx := context.Background()
	marker := filepath.Join(t.TempDir(), "dispatch.log")
	put(t, root, "evaluators/slow.py", fmt.Sprintf("import time\nfrom seven_evaluate import evaluator\n@evaluator(id='slow', metrics=['output.present'])\ndef evaluate(ctx):\n with open(%q,'a') as out: out.write(ctx['case']['inputs']+'\\n')\n time.sleep(.4)\n return []\n", marker))
	put(t, root, "plans/production.yaml", "schema_version: evaluate.plan/v1\nid: production\nmode: stream\nevaluators: [evaluator:slow]\nstream:\n  unit: trace\n")
	a, b := liveEvent("first", 1, "ok"), liveEvent("second", 1, "ok")
	a.Case.Inputs = "first"
	b.Case.Inputs = "second"
	if _, err := m.Ingest(ctx, IngestBatch{PlanID: "production", Events: []ProductionEvent{a, b}}); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { m.scorePending(ctx); close(done) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("evaluator never started")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := m.SetPaused(ctx, "production", true); err != nil {
		t.Fatal(err)
	}
	<-done
	bytes, err := os.ReadFile(marker)
	if err != nil || string(bytes) != "first\n" {
		t.Fatalf("pause boundary %q %v", bytes, err)
	}
	if _, err := m.SetPaused(ctx, "production", false); err != nil {
		t.Fatal(err)
	}
	m.scorePending(ctx)
	bytes, err = os.ReadFile(marker)
	if err != nil || string(bytes) != "first\nsecond\n" {
		t.Fatalf("ordered resume %q %v", bytes, err)
	}
}

func TestOTLPMissingRootTimeoutThenLateRevision(t *testing.T) {
	e, m, _ := newStoppedLive(t)
	ctx := context.Background()
	body, err := os.ReadFile(filepath.Join(e.Worker.Home, "protocol/examples/otlp-openinference.json"))
	if err != nil {
		t.Fatal(err)
	}
	var req otlpRequest
	if err = decodeJSON(body, &req); err != nil {
		t.Fatal(err)
	}
	root, child := req.ResourceSpans[0].ScopeSpans[0].Spans[0], req.ResourceSpans[0].ScopeSpans[0].Spans[1]
	req.ResourceSpans[0].ScopeSpans[0].Spans = []otlpSpan{child}
	body, _ = json.Marshal(req)
	if _, err = m.IngestOTLP(ctx, body); err != nil {
		t.Fatal(err)
	}
	for _, a := range testLiveAssemblies(t, m) {
		a.FirstAt = time.Now().Add(-121 * time.Second)
		setTestAssembly(t, m, a)
	}
	m.flushOTLP(ctx)
	m.scorePending(ctx)
	if len(testLiveRecords(t, m)) != 1 {
		t.Fatal("incomplete evidence not emitted at timeout")
	}
	for _, r := range testLiveRecords(t, m) {
		if r.Case.Metrics[0].Status != "not_computable" || r.Case.Metrics[0].Value != nil {
			t.Fatal("missing root scored as zero")
		}
	}
	req.ResourceSpans[0].ScopeSpans[0].Spans = []otlpSpan{root}
	body, _ = json.Marshal(req)
	if _, err = m.IngestOTLP(ctx, body); err != nil {
		t.Fatal(err)
	}
	for _, a := range testLiveAssemblies(t, m) {
		a.UpdatedAt = time.Now().Add(-6 * time.Second)
		setTestAssembly(t, m, a)
	}
	m.flushOTLP(ctx)
	m.scorePending(ctx)
	report, err := liveReport(t, m, ctx, "production", time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(testLiveRecords(t, m)) != 2 || len(report.Windows) != 1 || report.Current.Total != 1 || report.Current.Scope.Scored != 1 {
		t.Fatal("late root did not replace incomplete evidence")
	}
}

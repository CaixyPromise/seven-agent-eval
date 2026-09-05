package core

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestProjectionFailurePreservesDurableCases(t *testing.T) {
	e, s, _ := fixture(t)
	r, err := e.Prepare(context.Background(), "smoke")
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(s.Root, "runs", r.ID, "result.json")
	if err = os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(p, 0700); err != nil {
		t.Fatal(err)
	}
	r.Cases = append(r.Cases, CaseResult{})
	r.Completed = 1
	err = s.Commit(&r, "case.completed", r.Cases[0])
	var projection *ProjectionError
	if !errors.As(err, &projection) {
		t.Fatalf("expected durable projection failure, got %v", err)
	}
	s.Close()
	if err = os.Remove(p); err != nil {
		t.Fatal(err)
	}
	recovered, err := NewStore(s.Root)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	got, err := recovered.Get(r.ID)
	if err != nil || got.Completed != 1 || len(got.Cases) != 1 || got.Status != "partial" {
		t.Fatalf("lost journal result: %+v %v", got, err)
	}
}

func TestLargeIntegerPreservedAcrossWorkerAndBundle(t *testing.T) {
	e, _, root := fixture(t)
	put(t, root, "datasets/sample/cases.jsonl", `{"id":"large","inputs":{},"response":9007199254740993,"references":{"exact":9007199254740992}}`+"\n")
	r := runPlan(t, e, "smoke")
	if getMetric(t, r, "output.exact").Value != false {
		t.Fatal("distinct large integers compared equal")
	}
	b, _ := json.Marshal(r)
	if !bytes.Contains(b, []byte("9007199254740993")) {
		t.Fatal("response precision lost")
	}
}

func home(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
}
func put(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(p, []byte(content), 0600); e != nil {
		t.Fatal(e)
	}
}
func fixture(t *testing.T) (*Engine, *Store, string) {
	t.Helper()
	root := t.TempDir()
	if e := InitPackage(root); e != nil {
		t.Fatal(e)
	}
	s, e := NewStore(filepath.Join(root, ".seven-evaluate"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	return &Engine{Root: root, Store: s, Worker: NewWorker(home(t))}, s, root
}
func runPlan(t *testing.T, e *Engine, plan string) Run {
	t.Helper()
	r, err := e.Prepare(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	r, err = e.Execute(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func getMetric(t *testing.T, r Run, id string) Metric {
	t.Helper()
	for _, m := range r.Cases[0].Metrics {
		if m.MetricID == id {
			return m
		}
	}
	t.Fatalf("missing %s", id)
	return Metric{}
}
func TestOfflineDefaultsBundleAndRebuild(t *testing.T) {
	e, s, root := fixture(t)
	r := runPlan(t, e, "smoke")
	if r.Status != "completed" || len(r.Outputs) != 1 || len(r.Conclusions) != 1 {
		t.Fatalf("unexpected run: %+v", r)
	}
	if getMetric(t, r, "output.exact").Value != true {
		t.Fatal("exact check failed")
	}
	if e := ValidateRun(r); e != nil {
		t.Fatal(e)
	}
	dest := filepath.Join(t.TempDir(), "demo.seven.eval")
	if err := Export(r, dest); err != nil {
		t.Fatal(err)
	}
	imported, err := ImportFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if Digest(imported) != Digest(r) {
		t.Fatal("roundtrip differs")
	}
	if err = Export(r, dest); err == nil {
		t.Fatal("must not overwrite existing export")
	}
	s.Close()
	if err = os.Remove(filepath.Join(root, ".seven-evaluate", "index.sqlite")); err != nil {
		t.Fatal(err)
	}
	rebuilt, err := NewStore(filepath.Join(root, ".seven-evaluate"))
	if err != nil {
		t.Fatal(err)
	}
	defer rebuilt.Close()
	again, err := rebuilt.Get(r.ID)
	if err != nil || Digest(again) != Digest(r) {
		t.Fatal("projection rebuild mismatch", err)
	}
}
func TestStageOmittedReplacementAndDisable(t *testing.T) {
	e, _, root := fixture(t)
	put(t, root, "plans/raw.yaml", "schema_version: evaluate.plan/v1\nid: raw\ndataset: dataset:sample\nevaluators: []\nprocessors: []\nconclusions: []\n")
	raw := runPlan(t, e, "raw")
	if len(raw.Cases[0].Metrics) != 0 || len(raw.Outputs) != 0 || len(raw.Conclusions) != 0 {
		t.Fatal("explicit disabled ignored")
	}
	put(t, root, "processors/team.py", "from seven_evaluate import processor\n@processor(id='team',version='1')\ndef process(ctx):\n return {'processor':'team@1','data':{'replaced':True},'input_digest':ctx['input_digest']}\n")
	put(t, root, "plans/custom.yaml", "schema_version: evaluate.plan/v1\nid: custom\ndataset: dataset:sample\nprocessors: [processor:team]\n")
	custom := runPlan(t, e, "custom")
	if len(custom.Outputs) != 1 || custom.Outputs[0].Processor != "team@1" || custom.Conclusions[0].Code != "dependency_missing" {
		t.Fatal("custom replacement fell back")
	}
}
func TestHTTPInputsOnlyAndReplayNeverInvokes(t *testing.T) {
	e, _, root := fixture(t)
	var calls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		b, _ := io.ReadAll(r.Body)
		if string(b) != `{"question":"Say hello"}` {
			t.Errorf("unexpected subject input %s", b)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"answer":"Hello","contexts":[]}`)
	}))
	defer target.Close()
	put(t, root, "adapters/http.yaml", fmt.Sprintf("id: http\ntype: http\nendpoint: %s\nrequest:\n  question: $.question\nresponse_path: $.answer\ncontexts_path: $.contexts\n", target.URL))
	put(t, root, "plans/invoke.yaml", "schema_version: evaluate.plan/v1\nid: invoke\ndataset: dataset:sample\nmode: invoke\nsubject: adapter:http\n")
	r := runPlan(t, e, "invoke")
	if r.Status != "completed" || calls.Load() != 1 {
		t.Fatal("invoke failed")
	}
	if getMetric(t, r, "latency.ms").Status != "observed" {
		t.Fatal("latency missing")
	}
	runPlan(t, e, "smoke")
	if calls.Load() != 1 {
		t.Fatal("replay invoked subject")
	}
}
func TestHTTPErrorIsNotZero(t *testing.T) {
	e, _, root := fixture(t)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "secret upstream body", 500) }))
	defer target.Close()
	put(t, root, "adapters/http.yaml", fmt.Sprintf("id: http\ntype: http\nendpoint: %s\n", target.URL))
	put(t, root, "plans/invoke.yaml", "schema_version: evaluate.plan/v1\nid: invoke\ndataset: dataset:sample\nmode: invoke\nsubject: adapter:http\n")
	r := runPlan(t, e, "invoke")
	m := getMetric(t, r, "output.present")
	if r.Status != "partial" || m.Status != "not_computable" || m.Value != nil {
		t.Fatal("HTTP failure misclassified")
	}
	body, _ := json.Marshal(r)
	if bytes.Contains(body, []byte("secret upstream body")) {
		t.Fatal("provider body leaked")
	}
}
func TestCancelledInvocationRetainsPartialReport(t *testing.T) {
	e, _, root := fixture(t)
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		started <- struct{}{}
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer target.Close()
	defer close(release)
	put(t, root, "adapters/http.yaml", fmt.Sprintf("id: http\ntype: http\nendpoint: %s\n", target.URL))
	put(t, root, "plans/invoke.yaml", "schema_version: evaluate.plan/v1\nid: invoke\ndataset: dataset:sample\nmode: invoke\nsubject: adapter:http\n")
	r, err := e.Prepare(context.Background(), "invoke")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { <-started; cancel() }()
	r, err = e.Execute(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "cancelled" || len(r.Outputs) != 1 {
		t.Fatalf("partial report missing: %s", r.Status)
	}
	if len(r.Cases[0].Metrics) != 0 {
		t.Fatal("cancelled unscored case got metrics")
	}
	if err = Export(r, filepath.Join(t.TempDir(), "partial.seven.eval")); err != nil {
		t.Fatal(err)
	}
}
func TestWorkerTimeoutAndCrash(t *testing.T) {
	w := NewWorker(home(t))
	root := t.TempDir()
	for _, tc := range []struct {
		name, body string
		timeout    time.Duration
	}{
		{"hang", "import time; time.sleep(60)", 150 * time.Millisecond}, {"crash", "import os; os._exit(7)", time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			put(t, root, tc.name+".py", "from seven_evaluate import evaluator\n@evaluator(id='x',version='1')\ndef ev(ctx):\n "+tc.body+"\n")
			ctx, cancel := context.WithTimeout(context.Background(), tc.timeout)
			defer cancel()
			var out any
			if err := w.Call(ctx, "evaluator/evaluate", map[string]any{"path": filepath.Join(root, tc.name+".py"), "context": map[string]any{}}, &out); err == nil {
				t.Fatal("worker failure ignored")
			}
		})
	}
}
func TestRedactionDoesNotChangeScore(t *testing.T) {
	e, _, root := fixture(t)
	put(t, root, "datasets/sample/cases.jsonl", `{"id":"a","inputs":"x","response":{"secret":"sensitive"},"references":{"exact":{"secret":"sensitive"}}}`+"\n")
	r := runPlan(t, e, "smoke")
	if getMetric(t, r, "output.exact").Value != true {
		t.Fatal("redaction changed score")
	}
	if !r.Cases[0].RedactionApplied {
		t.Fatal("redaction provenance missing")
	}
	b, _ := json.Marshal(r)
	if bytes.Contains(b, []byte("sensitive")) {
		t.Fatal("redacted content leaked")
	}
}
func TestEvaluatorFailureDoesNotContaminateNext(t *testing.T) {
	e, _, root := fixture(t)
	put(t, root, "evaluators/broken.py", "from seven_evaluate import evaluator\n@evaluator(id='broken',version='1')\ndef ev(ctx):\n raise RuntimeError('broken')\n")
	put(t, root, "plans/mixed.yaml", "schema_version: evaluate.plan/v1\nid: mixed\ndataset: dataset:sample\nevaluators: [evaluator:broken, evaluator:basic]\n")
	r := runPlan(t, e, "mixed")
	if r.Status != "partial" || getMetric(t, r, "output.exact").Value != true {
		t.Fatal("independent evaluator contaminated")
	}
}
func TestLocalHelperFrozenAtPreparation(t *testing.T) {
	e, _, root := fixture(t)
	put(t, root, "processors/_helper.py", "VALUE='original'\n")
	put(t, root, "processors/team.py", "from seven_evaluate import processor\nfrom _helper import VALUE\n@processor(id='team',version='1')\ndef process(ctx):\n return {'processor':'team@1','data':{'version':VALUE},'input_digest':ctx['input_digest']}\n")
	put(t, root, "plans/custom.yaml", "schema_version: evaluate.plan/v1\nid: custom\ndataset: dataset:sample\nprocessors: [processor:team]\n")
	r, err := e.Prepare(context.Background(), "custom")
	if err != nil {
		t.Fatal(err)
	}
	put(t, root, "processors/_helper.py", "VALUE='changed'\n")
	r, err = e.Execute(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if r.Outputs[0].Data["version"] != "original" {
		t.Fatal("helper was not frozen")
	}
	if r.Effective.Files["package/processors/_helper.py"] == "" {
		t.Fatal("helper missing from provenance")
	}
}
func TestDiscoveryMovementDuplicatesHelpersAndInvalid(t *testing.T) {
	e, _, root := fixture(t)
	put(t, root, "evaluators/_ignored.py", "raise RuntimeError('must not import')\n")
	put(t, root, "tests/evaluators/not_asset.py", "raise RuntimeError('must not import')\n")
	put(t, root, "plans/broken.yaml", "schema_version: evaluate.plan/v1\nid: broken\ndataset: dataset:missing\n")
	d, err := Discover(context.Background(), root, e.Worker)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range d.Assets {
		if strings.Contains(a.Path, "ignored") || strings.Contains(a.Path, "not_asset") {
			t.Fatal("helper discovered")
		}
	}
	if a, err := Lookup(d, "plan:broken", "plan"); err == nil || a.Status != "invalid" {
		t.Fatal("broken reference not shown")
	}
	dest := filepath.Join(t.TempDir(), "moved")
	if err = os.Rename(root, dest); err != nil {
		t.Fatal(err)
	}
	moved, err := Discover(context.Background(), dest, e.Worker)
	if err != nil {
		t.Fatal(err)
	}
	if Digest(d) != Digest(moved) {
		t.Fatal("discovery identity depends on absolute path")
	}
	put(t, dest, "plans/smoke/plan.yaml", "schema_version: evaluate.plan/v1\nid: smoke\ndataset: dataset:sample\n")
	dup, err := Discover(context.Background(), dest, e.Worker)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Lookup(dup, "smoke", "plan"); err == nil {
		t.Fatal("duplicate plan accepted")
	}
}
func TestDatasetRejectsNullStagesDuplicateCasesAndMissingReplay(t *testing.T) {
	for _, tc := range []struct{ name, plan, data string }{
		{"null-stage", "processors: null\n", ""},
		{"duplicate", "", "{\"id\":\"a\",\"inputs\":\"x\",\"response\":\"ok\"}\n{\"id\":\"a\",\"inputs\":\"y\",\"response\":\"ok\"}\n"},
		{"missing-response", "", "{\"id\":\"a\",\"inputs\":\"x\"}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, _, root := fixture(t)
			if tc.plan != "" {
				put(t, root, "plans/smoke.yaml", "schema_version: evaluate.plan/v1\nid: smoke\ndataset: dataset:sample\n"+tc.plan)
			}
			if tc.data != "" {
				put(t, root, "datasets/sample/cases.jsonl", tc.data)
			}
			if _, err := e.Prepare(context.Background(), "smoke"); err == nil {
				t.Fatal("invalid input accepted")
			}
		})
	}
}
func TestVerifiedImportRejectsBadRelationshipsAndTampering(t *testing.T) {
	e, _, _ := fixture(t)
	r := runPlan(t, e, "smoke")
	for _, tc := range []struct {
		name   string
		mutate func(*Run)
	}{
		{"bad-case-status", func(r *Run) { r.Cases[0].Status = "made-up" }},
		{"completed-error", func(r *Run) { r.Cases[0].Status = "error" }},
		{"missing-evidence", func(r *Run) { r.Cases[0].Metrics[0].EvidenceRefs = []string{"nonexistent"} }},
		{"duplicate-metric", func(r *Run) { r.Cases[0].Metrics = append(r.Cases[0].Metrics, r.Cases[0].Metrics[0]) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, _ := json.Marshal(r)
			var v Run
			json.Unmarshal(b, &v)
			tc.mutate(&v)
			if ValidateRun(v) == nil {
				t.Fatal("bad relationship accepted")
			}
		})
	}
	dest := filepath.Join(t.TempDir(), "ok.seven.eval")
	if err := Export(r, dest); err != nil {
		t.Fatal(err)
	}
	source, _ := zip.OpenReader(dest)
	defer source.Close()
	for _, kind := range []string{"tamper", "traversal", "duplicate"} {
		t.Run(kind, func(t *testing.T) {
			var buf bytes.Buffer
			writer := zip.NewWriter(&buf)
			for _, f := range source.File {
				rd, _ := f.Open()
				b, _ := io.ReadAll(rd)
				rd.Close()
				if kind == "tamper" && f.Name == "run.json" {
					b = []byte("{}")
				}
				entry, _ := writer.Create(f.Name)
				entry.Write(b)
			}
			if kind == "traversal" {
				w, _ := writer.Create("../escape")
				w.Write([]byte("x"))
			}
			if kind == "duplicate" {
				w, _ := writer.Create("manifest.json")
				w.Write([]byte("{}"))
			}
			writer.Close()
			if _, err := Import(bytes.NewReader(buf.Bytes()), int64(buf.Len())); err == nil {
				t.Fatal("unsafe bundle accepted")
			}
		})
	}
}
func TestUnfinishedRunRecoveryNeverReinvokes(t *testing.T) {
	e, s, root := fixture(t)
	r, err := e.Prepare(context.Background(), "smoke")
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	rebuilt, err := NewStore(filepath.Join(root, ".seven-evaluate"))
	if err != nil {
		t.Fatal(err)
	}
	defer rebuilt.Close()
	recovered, err := rebuilt.Get(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Status != "partial" || recovered.Completed != 0 {
		t.Fatal("unfinished run was resumed or marked completed")
	}
}

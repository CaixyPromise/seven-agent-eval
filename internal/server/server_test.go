package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"seven-agent-eval/internal/core"
	"strings"
	"testing"
	"time"
)

func setup(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	_, f, _, _ := runtime.Caller(0)
	home := filepath.Clean(filepath.Join(filepath.Dir(f), "../.."))
	root := t.TempDir()
	if e := core.InitPackage(root); e != nil {
		t.Fatal(e)
	}
	store, e := core.NewStore(filepath.Join(root, ".seven-evaluate"))
	if e != nil {
		t.Fatal(e)
	}
	app := New(&core.Engine{Root: root, Store: store, Worker: core.NewWorker(home)}, filepath.Join(home, "web/dist"))
	target := httptest.NewServer(app.Handler())
	t.Cleanup(func() { target.Close(); app.Shutdown(); store.Close() })
	return app, target
}
func TestBrowserAPIExecuteSSEAndReport(t *testing.T) {
	_, s := setup(t)
	response, e := http.Get(s.URL + "/api/assets")
	if e != nil {
		t.Fatal(e)
	}
	var d core.Discovery
	json.NewDecoder(response.Body).Decode(&d)
	response.Body.Close()
	if len(d.Assets) < 6 {
		t.Fatal("discovery incomplete")
	}
	response, e = http.Post(s.URL+"/api/runs", "application/json", strings.NewReader(`{"plan":"smoke"}`))
	if e != nil {
		t.Fatal(e)
	}
	if response.StatusCode != 202 {
		b, _ := io.ReadAll(response.Body)
		response.Body.Close()
		t.Fatalf("start: %s", b)
	}
	var started core.Run
	json.NewDecoder(response.Body).Decode(&started)
	response.Body.Close()
	deadline := time.Now().Add(10 * time.Second)
	var run core.Run
	for time.Now().Before(deadline) {
		response, e = http.Get(s.URL + "/api/runs/" + started.ID)
		if e != nil {
			t.Fatal(e)
		}
		decoder := json.NewDecoder(response.Body)
		decoder.UseNumber()
		if err := decoder.Decode(&run); err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if run.Status == "completed" {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	if run.Status != "completed" || len(run.Outputs) != 1 {
		t.Fatalf("run didn't complete: %+v", run)
	}
	response, e = http.Get(s.URL + "/api/runs/" + run.ID + "/events")
	if e != nil {
		t.Fatal(e)
	}
	data, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if !bytes.Contains(data, []byte("event: snapshot")) || !bytes.Contains(data, []byte("completed")) {
		t.Fatal("SSE doesn't carry current state")
	}
	response, e = http.Get(s.URL + "/api/runs/" + run.ID + "/report")
	if e != nil {
		t.Fatal(e)
	}
	html, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if !bytes.Contains(html, []byte("Catalogs")) || !bytes.Contains(html, []byte("Hello")) {
		t.Fatal("default report missing")
	}
	response, e = http.Get(s.URL + "/api/runs/" + run.ID + "/export")
	if e != nil {
		t.Fatal(e)
	}
	bundle, _ := io.ReadAll(response.Body)
	response.Body.Close()
	imported, e := core.Import(bytes.NewReader(bundle), int64(len(bundle)))
	if e != nil {
		t.Fatal(e)
	}
	if core.Digest(imported) != core.Digest(run) {
		t.Fatal("API export differs")
	}
}
func TestImportAndReadNeverStartPython(t *testing.T) {
	app, _ := setup(t)
	run, err := app.Engine.Prepare(context.Background(), "smoke")
	if err != nil {
		t.Fatal(err)
	}
	run, err = app.Engine.Execute(context.Background(), run)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "demo.seven.eval")
	if err = core.Export(run, p); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	target, s := setup(t)
	target.Engine.Worker.Python = "/does/not/exist"
	response, err := http.Post(s.URL+"/api/import", "application/zip", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 {
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		t.Fatalf("import failed: %s", body)
	}
	response.Body.Close()
	response, err = http.Get(s.URL + "/api/runs/" + run.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var imported core.Run
	if err = json.NewDecoder(response.Body).Decode(&imported); err != nil {
		t.Fatal(err)
	}
	if imported.ID != run.ID {
		t.Fatal("import identity lost")
	}
}
func TestCrossOriginAndHostRejected(t *testing.T) {
	app, _ := setup(t)
	for _, tc := range []struct{ host, origin, site string }{
		{"evil.example", "", ""}, {"127.0.0.1:7431", "https://evil.example", ""}, {"127.0.0.1:7431", "", "cross-site"},
	} {
		req := httptest.NewRequest("POST", "http://127.0.0.1:7431/api/runs", strings.NewReader(`{"plan":"smoke"}`))
		req.Host = tc.host
		req.Header.Set("Origin", tc.origin)
		req.Header.Set("Sec-Fetch-Site", tc.site)
		rec := httptest.NewRecorder()
		app.Handler().ServeHTTP(rec, req)
		if rec.Code != 403 {
			t.Fatalf("unsafe request accepted: %d", rec.Code)
		}
	}
}

func TestLiveIngestPauseOTLPAndWindowExport(t *testing.T) {
	_, f, _, _ := runtime.Caller(0)
	home := filepath.Clean(filepath.Join(filepath.Dir(f), "../.."))
	root := t.TempDir()
	if err := core.InitPackage(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "plans", "production.yaml"), []byte("schema_version: evaluate.plan/v1\nid: production\nmode: stream\nstream:\n  unit: trace\n"), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := core.NewStore(filepath.Join(root, ".seven-evaluate"))
	if err != nil {
		t.Fatal(err)
	}
	app := New(&core.Engine{Root: root, Store: store, Worker: core.NewWorker(home)}, filepath.Join(home, "web/dist"))
	target := httptest.NewServer(app.Handler())
	t.Cleanup(func() { target.Close(); app.Shutdown(); store.Close() })

	response, err := http.Get(target.URL + "/api/live/plans")
	if err != nil || response.StatusCode != 200 {
		t.Fatalf("plans: %v %v", response.Status, err)
	}
	response.Body.Close()
	batch := fmt.Sprintf(`{"plan_id":"production","events":[{"schema_version":"evaluate.event/v1","source":"test","project":"demo","event_id":"one","event_time":%q,"trace_id":"trace-one","evidence_revision":1,"case":{"id":"one","inputs":{"question":"hello"},"references":{"exact":"hello"}},"result":{"response":"hello","latency_ms":12}}]}`, time.Now().UTC().Format(time.RFC3339Nano))
	response, err = http.Post(target.URL+"/api/ingest/events", "application/json", strings.NewReader(batch))
	if err != nil || response.StatusCode != http.StatusAccepted {
		t.Fatalf("ingest: %v %v", response.Status, err)
	}
	response.Body.Close()
	response, err = http.Post(target.URL+"/api/live/plans/production/pause", "application/json", nil)
	if err != nil || response.StatusCode != 200 {
		t.Fatalf("pause: %v %v", response.Status, err)
	}
	response.Body.Close()
	response, err = http.Post(target.URL+"/api/live/plans/production/resume", "application/json", nil)
	if err != nil || response.StatusCode != 200 {
		t.Fatalf("resume: %v %v", response.Status, err)
	}
	response.Body.Close()
	deadline := time.Now().Add(6 * time.Second)
	var report core.LiveReport
	for time.Now().Before(deadline) {
		response, err = http.Get(target.URL + "/api/live/plans/production/report?range=24h")
		if err != nil {
			t.Fatal(err)
		}
		_ = json.NewDecoder(response.Body).Decode(&report)
		response.Body.Close()
		if report.Current != nil && report.Current.Scope.Scored == 1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if report.Current == nil || report.Current.Scope.Scored != 1 {
		t.Fatalf("report not scored: %+v", report)
	}
	response, err = http.Get(target.URL + "/api/live/windows/" + report.Current.ID + "/export")
	if err != nil || response.StatusCode != 200 {
		t.Fatalf("export: %v %v", response.Status, err)
	}
	bundle, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if _, err = core.Import(bytes.NewReader(bundle), int64(len(bundle))); err != nil {
		t.Fatal(err)
	}
	response, err = http.Post(target.URL+"/v1/traces", "application/x-protobuf", strings.NewReader("no"))
	if err != nil || response.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("unsupported OTLP: %v %v", response.Status, err)
	}
	response.Body.Close()
}

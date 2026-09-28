package core

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSevenEpisodeIsolationAndFailure(t *testing.T) {
	e, _, root := fixture(t)
	var mu sync.Mutex
	created := 0
	calls := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if strings.HasSuffix(r.URL.Path, "/sessions") {
			created++
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"data":{"id":%d}}`, created)
			return
		}
		calls = append(calls, r.URL.Path)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: conversation.started\ndata: {\"ops\":[{\"op\":\"part.replace\",\"part\":{\"id\":\"m\",\"type\":\"message\",\"text\":\"part\"}}]}\n\n")
		fmt.Fprint(w, "event: conversation.patch\ndata: {\"ops\":[{\"op\":\"part.append_text\",\"partId\":\"m\",\"text\":\"ial\"},{\"op\":\"part.replace\",\"part\":{\"id\":\"t\",\"type\":\"tool\",\"toolCallId\":\"t1\",\"toolName\":\"search\",\"state\":\"completed\",\"input\":{\"query\":\"q\"},\"output\":{\"ok\":true}}}]}\n\n")
		if len(calls) == 2 {
			fmt.Fprint(w, "event: conversation.failed\ndata: {}\n\n")
		} else {
			fmt.Fprint(w, "event: unknown.safe-type\ndata: {}\n\nevent: conversation.completed\ndata: {}\n\n")
		}
	}))
	defer server.Close()
	put(t, root, "adapters/seven.yaml", "id: seven\ntype: seven-workspace\nendpoint: "+server.URL+"/api/v1\nworkspace_id: '1'\n")
	put(t, root, "plans/seven.yaml", "schema_version: evaluate.plan/v1\nid: seven\nmode: invoke\ndataset: dataset:sample\nsubject: adapter:seven\n")
	input := `{"id":"ep1","inputs":{},"episode":{"id":"ep1","turns":[{"id":"a","case":{"id":"a","inputs":{"text":"a"}}},{"id":"b","case":{"id":"b","inputs":{"text":"b"}}},{"id":"c","case":{"id":"c","inputs":{"text":"c"}}}]}}` + "\n" + `{"id":"ep2","inputs":{},"episode":{"id":"ep2","turns":[{"id":"a","case":{"id":"a","inputs":{"text":"d"}}}]}}` + "\n"
	put(t, root, "datasets/sample/cases.jsonl", input)
	r := runPlan(t, e, "seven")
	if r.Status != "partial" || r.Total != 4 || r.Completed != 3 || created != 2 || len(calls) != 3 {
		t.Fatalf("episode state: %+v, created=%d calls=%v", r, created, calls)
	}
	if calls[0] != calls[1] || calls[1] == calls[2] {
		t.Fatal("session isolation failed")
	}
	if r.Cases[1].Result.Response != "partial" || len(r.Cases[0].Result.Tools) != 1 {
		t.Fatal("partial/tool evidence lost")
	}
	if len(r.Episodes) != 2 || r.Episodes[0].Status != "partial" || r.Episodes[1].Status != "completed" {
		t.Fatal(r.Episodes)
	}
	if err := Export(r, root+"/episode.seven.eval"); err != nil {
		t.Fatal(err)
	}
}

func TestSevenMissingTerminalAndCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: conversation.patch\ndata: {}\n\n")
	}))
	defer server.Close()
	ctx := context.WithValue(context.Background(), subjectSessionKey{}, "1")
	_, err := invokeSeven(ctx, HTTPConfig{Endpoint: server.URL, WorkspaceID: "1"}, Case{Inputs: map[string]any{"text": "q"}})
	if err == nil {
		t.Fatal("EOF inferred success")
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = invokeSeven(ctx, HTTPConfig{Endpoint: server.URL, WorkspaceID: "1"}, Case{Inputs: map[string]any{"text": "q"}}); err == nil {
		t.Fatal("cancel ignored")
	}
}

func TestSevenTimeoutRetainsLatestPartialMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: conversation.patch\ndata: {\"ops\":[{\"op\":\"part.replace\",\"part\":{\"id\":\"old\",\"type\":\"message\",\"text\":\"intermediate\"}},{\"op\":\"part.replace\",\"part\":{\"id\":\"final\",\"type\":\"message\",\"text\":\"partial\"}}]}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.WithValue(context.Background(), subjectSessionKey{}, "1"), 100*time.Millisecond)
	defer cancel()
	result, err := invokeSeven(ctx, HTTPConfig{Endpoint: server.URL, WorkspaceID: "1"}, Case{Inputs: map[string]any{"text": "q"}})
	if err == nil || result.Response != "partial" {
		t.Fatalf("timeout/partial answer: %+v %v", result, err)
	}
}

func TestPythonEpisodeLifecycleOwnsOnlyCreatedSessions(t *testing.T) {
	e, _, root := fixture(t)
	log := root + "/lifecycle.log"
	script := `from seven_evaluate import subject
import pathlib
log=pathlib.Path(` + strconv.Quote(log) + `)
def record(value):
    with log.open("a") as output: output.write(value+"\n")
def create(ctx):
    record("create")
    return {"id":"owned"}
def cleanup(ctx): record("cleanup:"+ctx["session"]["id"])
@subject(id="session", session_create=create, session_cleanup=cleanup)
def invoke(ctx):
    record("invoke:"+ctx["session"]["id"])
    return {"response":ctx["session"]["id"]}
`
	put(t, root, "adapters/session.py", script)
	put(t, root, "plans/session.yaml", "schema_version: evaluate.plan/v1\nid: session\nmode: invoke\ndataset: dataset:sample\nsubject: adapter:session\nparameters:\n  cleanup_sessions: true\n")
	put(t, root, "datasets/sample/cases.jsonl", `{"id":"ep","inputs":{},"episode":{"id":"ep","turns":[{"id":"a","case":{"id":"a","inputs":{}}},{"id":"b","case":{"id":"b","inputs":{}}}]}}`+"\n")
	r := runPlan(t, e, "session")
	if r.Status != "completed" || r.Cases[1].Result.Response != "owned" {
		t.Fatalf("lifecycle result: %+v", r)
	}
	b, err := os.ReadFile(log)
	if err != nil || string(b) != "create\ninvoke:owned\ninvoke:owned\ncleanup:owned\n" {
		t.Fatalf("lifecycle order: %s %v", b, err)
	}
}

func TestEpisodeInputsFailBeforeCallingSubject(t *testing.T) {
	e, _, root := fixture(t)
	put(t, root, "adapters/seven.yaml", "id: seven\ntype: seven-workspace\nendpoint: http://127.0.0.1:1/api/v1\nworkspace_id: '1'\n")
	put(t, root, "plans/preflight.yaml", "schema_version: evaluate.plan/v1\nid: preflight\nmode: invoke\ndataset: dataset:sample\nsubject: adapter:seven\n")
	put(t, root, "datasets/sample/cases.jsonl", `{"id":"ep","inputs":{},"episode":{"id":"ep","turns":[{"id":"a","case":{"id":"a","inputs":{"text":"q"},"references":{"qrels":{"d":-1}}}}]}}`+"\n")
	if _, err := e.Prepare(context.Background(), "preflight"); err == nil || !strings.Contains(err.Error(), "qrel relevance") {
		t.Fatalf("invalid nested reference reached Subject: %v", err)
	}
	put(t, root, "plans/preflight.yaml", "schema_version: evaluate.plan/v1\nid: preflight\nmode: replay\ndataset: dataset:sample\n")
	put(t, root, "datasets/sample/cases.jsonl", `{"id":"ep","inputs":{},"episode":{"id":"ep","turns":[{"id":"a","case":{"id":"a","inputs":{"text":"q"}}}]}}`+"\n")
	if _, err := e.Prepare(context.Background(), "preflight"); err == nil || !strings.Contains(err.Error(), "replay requires") {
		t.Fatalf("missing historical Turn answer became zero: %v", err)
	}
}

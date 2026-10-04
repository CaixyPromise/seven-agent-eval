package core

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestPreparedInputIsAuthoritative(t *testing.T) {
	e, s, root := fixture(t)
	r, err := e.Prepare(context.Background(), "smoke")
	if err != nil {
		t.Fatal(err)
	}
	frozen := filepath.Join(s.Root, "runs", r.ID, "inputs", "cases.jsonl")
	if _, err := os.Stat(frozen); err != nil {
		t.Fatal("normalized input not frozen", err)
	}
	put(t, root, "datasets/sample/cases.jsonl", "invalid replacement")
	// Execution must not parse the copied raw data either.
	put(t, filepath.Join(s.Root, "runs", r.ID, "source"), "datasets/sample/cases.jsonl", "invalid copied data")
	got, err := executeFixture(t, e, context.Background(), r)
	if err != nil || got.Completed != r.Total {
		t.Fatalf("frozen execution: %v %+v", err, got)
	}
}

func TestLateInputFailureNeverInvokesSubject(t *testing.T) {
	e, s, root := fixture(t)
	var calls atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.Write([]byte(`{"answer":"ok"}`)) }))
	defer target.Close()
	put(t, root, "adapters/target.yaml", "id: target\ntype: http\nendpoint: "+target.URL+"\nresponse_path: $.answer\n")
	put(t, root, "plans/invoke.yaml", "schema_version: evaluate.plan/v1\nid: invoke\nmode: invoke\ndataset: dataset:sample\nsubject: adapter:target\n")
	for _, tail := range []string{"invalid", `{"id":"case-0","inputs":{}}`, `{"id":"bad","inputs":{},"references":{"qrels":{"d":-1}}}`} {
		var body strings.Builder
		for i := 0; i < 100; i++ {
			fmt.Fprintf(&body, "{\"id\":\"case-%d\",\"inputs\":{}}\n", i)
		}
		body.WriteString(tail + "\n")
		put(t, root, "datasets/sample/cases.jsonl", body.String())
		if _, err := e.Prepare(context.Background(), "invoke"); err == nil {
			t.Fatalf("invalid final row accepted: %s", tail)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("preparation invoked subject")
	}
	runs, err := s.List()
	if err != nil || len(runs) != 0 {
		t.Fatalf("failed preparation published run: %v %+v", err, runs)
	}
	entries, err := os.ReadDir(filepath.Join(s.Root, "runs"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		files, err := os.ReadDir(filepath.Join(s.Root, "runs", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			if strings.HasPrefix(f.Name(), ".preparing-") {
				t.Fatal("scratch database leaked")
			}
		}
	}
}

func TestPreparedInputCancellationDoesNotPublish(t *testing.T) {
	e, s, root := fixture(t)
	d, err := Discover(context.Background(), root, e.Worker)
	if err != nil {
		t.Fatal(err)
	}
	effective, err := Resolve(root, d, "smoke")
	if err != nil {
		t.Fatal(err)
	}
	r := Run{ID: NewID(), Effective: effective}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.prepareInput(ctx, d, &r); err == nil {
		t.Fatal("cancelled preparation succeeded")
	}
	if _, err := os.Stat(filepath.Join(s.Root, "runs", r.ID, "inputs")); !os.IsNotExist(err) {
		t.Fatalf("cancelled input published: %v", err)
	}
}

func TestPreparedInputCrossEpisodeCollision(t *testing.T) {
	e, _, root := fixture(t)
	put(t, root, "datasets/sample/cases.jsonl", `{"id":"parent","inputs":{},"episode":{"id":"ep","turns":[{"id":"t1","case":{"id":"child","inputs":{},"response":"ok"}}]}}`+"\n"+`{"id":"ep:t1","inputs":{},"response":"ok"}`+"\n")
	if _, err := e.Prepare(context.Background(), "smoke"); err == nil {
		t.Fatal("normalized identity collision accepted")
	}
}

func TestPreparedReplayPreservesNullZeroFalse(t *testing.T) {
	e, _, root := fixture(t)
	put(t, root, "datasets/sample/cases.jsonl", `{"id":"null","inputs":{},"response":null}`+"\n"+`{"id":"zero","inputs":{},"response":0}`+"\n"+`{"id":"false","inputs":{},"response":false}`+"\n")
	r, err := e.Prepare(context.Background(), "smoke")
	if err != nil {
		t.Fatal(err)
	}
	input, err := e.openPreparedInput(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	decoder := json.NewDecoder(input)
	decoder.UseNumber()
	for _, expected := range []any{nil, json.Number("0"), false} {
		var c map[string]any
		if err := decoder.Decode(&c); err != nil {
			t.Fatal(err)
		}
		actual, present := c["response"]
		if !present || actual != expected {
			t.Fatalf("frozen value changed: %v %v expected %v", present, actual, expected)
		}
	}
	put(t, root, "datasets/sample/cases.jsonl", `{"id":"absent","inputs":{}}`+"\n")
	if _, err := e.Prepare(context.Background(), "smoke"); err == nil {
		t.Fatal("absent response accepted")
	}
}

func BenchmarkPreparedInput(b *testing.B) {
	for _, count := range []int{100, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			root := b.TempDir()
			f, err := os.Create(filepath.Join(root, "cases.jsonl"))
			if err != nil {
				b.Fatal(err)
			}
			for i := 0; i < count; i++ {
				if _, err := fmt.Fprintf(f, "{\"id\":\"c-%d\",\"inputs\":{},\"response\":\"%s\"}\n", i, strings.Repeat("x", 1024)); err != nil {
					b.Fatal(err)
				}
			}
			if err := f.Close(); err != nil {
				b.Fatal(err)
			}
			store, err := NewStore(filepath.Join(root, ".seven-evaluate"))
			if err != nil {
				b.Fatal(err)
			}
			defer store.Close()
			e := &Engine{Root: root, Store: store}
			d := Discovery{Assets: []Asset{{Kind: "dataset", ID: "data", Path: "cases.jsonl", Status: "valid"}}}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				r := Run{ID: NewID(), Effective: Effective{Plan: Plan{Dataset: "data", Mode: "replay"}}}
				if _, err := e.prepareInput(context.Background(), d, &r); err != nil {
					b.Fatal(err)
				}
				if r.Total != count {
					b.Fatal("input count changed")
				}
			}
		})
	}
}

func TestCorruptPreparedTailPreventsAllExecution(t *testing.T) {
	e, s, _ := fixture(t)
	r, err := e.Prepare(context.Background(), "smoke")
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(s.Root, "runs", r.ID, "inputs", "cases.jsonl"), os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString("invalid tail\n")
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	got, err := executeFixture(t, e, context.Background(), r)
	if err == nil || got.Completed != 0 {
		t.Fatalf("corrupt frozen input executed: %v %+v", err, got)
	}
}

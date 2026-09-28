package core

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestAllFormatFixturesNormalizeEqually(t *testing.T) {
	e, _, _ := fixture(t)
	e.Root = filepath.Join(home(t), "examples/rag-formats")
	d, err := Discover(context.Background(), e.Root, e.Worker)
	if err != nil {
		t.Fatal(err)
	}
	baseline := ""
	metricDigest := ""
	for _, format := range []string{"jsonl", "csv", "beir", "trec"} {
		p := Plan{Source: format, Mode: "replay"}
		if format == "csv" {
			p.Mapping = "csv"
		}
		cases, _, er := e.readSource(d, p)
		if er != nil {
			t.Fatalf("%s: %v", format, er)
		}
		if baseline == "" {
			baseline = Digest(cases)
		} else if Digest(cases) != baseline {
			t.Fatalf("%s normalized case differs", format)
		}
		r := runPlan(t, e, format)
		digest := Digest(r.Cases[0].Metrics)
		if metricDigest == "" {
			metricDigest = digest
		} else if digest != metricDigest {
			t.Fatalf("%s observations differ", format)
		}
	}
}

func TestSourceRAGHTTPAndPreflight(t *testing.T) {
	e, _, root := fixture(t)
	var calls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if _, ok := body["qrels"]; ok {
			t.Error("reference leaked")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"answer":"ok","retrieval":[{"id":"d2","text":"two"},{"id":"d1","text":"one"}],"citations":[{"document_id":"d1","start":0,"end":3}]}`))
	}))
	defer target.Close()
	put(t, root, "sources/demo.yaml", "id: demo\nversion: '1'\nformat: jsonl\npath: data/input.jsonl\n")
	put(t, root, "mappings/demo.yaml", "id: demo\nversion: '1'\nfields:\n  id: $.key\n  inputs: $.query\n  references: $.truth\n")
	put(t, root, "data/input.jsonl", `{"key":"q1","query":{"text":"question"},"truth":{"qrels":{"d1":2,"d2":1}}}`+"\n")
	put(t, root, "adapters/rag.yaml", "id: rag\ntype: http\nendpoint: "+target.URL+"\nresponse_path: $.answer\nretrieval_path: $.retrieval\ncitations_path: $.citations\n")
	put(t, root, "plans/rag.yaml", "schema_version: evaluate.plan/v1\nid: rag\nmode: invoke\npreset: builtin.rag-basic@1\nsource: source:demo\nmapping: mapping:demo\nsubject: adapter:rag\nparameters:\n  k: [1, 5]\n")
	r := runPlan(t, e, "rag")
	if r.DatasetRevision == nil || calls.Load() != 1 {
		t.Fatalf("missing provenance/call: %+v %d", r, calls.Load())
	}
	found := 0
	for _, m := range r.Cases[0].Metrics {
		if m.MetricID == "rag.precision" && m.Status == "observed" {
			found++
		}
	}
	if found != 2 {
		t.Fatalf("K identities lost: %+v", r.Cases[0].Metrics)
	}
	put(t, root, "data/input.jsonl", `{"key":"q1","query":{},"truth":{}}`+"\n"+`{"key":"q1","query":{},"truth":{}}`+"\n")
	if _, err := e.Prepare(context.Background(), "rag"); err == nil {
		t.Fatal("duplicate accepted")
	}
	if calls.Load() != 1 {
		t.Fatal("invalid input invoked subject")
	}
}

func TestJudgeCredentialRedactionUsesConfiguredEnvironment(t *testing.T) {
	e, _, _ := fixture(t)
	t.Setenv("CONTROLLED_JUDGE_KEY", "controlled-not-a-real-key")
	r := Run{Effective: Effective{Plan: Plan{Parameters: map[string]any{"judge": map[string]any{"auth_env": "CONTROLLED_JUDGE_KEY"}}}}}
	value := e.redactForRun(&r, map[string]any{"text": "echo controlled-not-a-real-key"})
	if value.(map[string]any)["text"] != "echo [REDACTED]" {
		t.Fatal("judge credential echoed into result")
	}
}

func TestRetrievalFormatsAndCSV(t *testing.T) {
	e, _, root := fixture(t)
	put(t, root, "data/queries.jsonl", `{"_id":"q1","text":"question"}`+"\n")
	put(t, root, "data/corpus.jsonl", `{"_id":"d1","text":"one"}`+"\n")
	put(t, root, "data/beir.tsv", "query-id\tcorpus-id\tscore\nq1\td1\t2\n")
	put(t, root, "data/trec.qrels", "q1 0 d1 2\n")
	put(t, root, "data/run.txt", "q1 Q0 d1 1 1.0 demo\n")
	for _, format := range []string{"beir", "trec"} {
		qrels := "data/beir.tsv"
		if format == "trec" {
			qrels = "data/trec.qrels"
		}
		put(t, root, "sources/"+format+".yaml", "id: "+format+"\nversion: '1'\nformat: "+format+"\nqueries: data/queries.jsonl\ncorpus: data/corpus.jsonl\nqrels: "+qrels+"\nrun: data/run.txt\nsplit: test\n")
	}
	d, err := Discover(context.Background(), root, e.Worker)
	if err != nil {
		t.Fatal(err)
	}
	a, _, err := e.readSource(d, Plan{Source: "beir", Mode: "replay"})
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := e.readSource(d, Plan{Source: "trec", Mode: "replay"})
	if err != nil {
		t.Fatal(err)
	}
	if Digest(a) != Digest(b) {
		t.Fatalf("normalization differs: %+v %+v", a, b)
	}
	put(t, root, "sources/csv.yaml", "id: csv\nversion: '1'\nformat: csv\npath: data/input.csv\n")
	put(t, root, "mappings/csv.yaml", "id: csv\nversion: '1'\nfields:\n  id: $.id\n  inputs: $.inputs\n  response: $.answer\nconversions:\n  inputs: json\n")
	put(t, root, "data/input.csv", "id,inputs,answer\nq1,\"{\"\"text\"\":\"\"question\"\"}\",\"answer, escaped\"\n")
	d, err = Discover(context.Background(), root, e.Worker)
	if err != nil {
		t.Fatal(err)
	}
	c, _, err := e.readSource(d, Plan{Source: "csv", Mapping: "csv", Mode: "replay"})
	if err != nil {
		t.Fatal(err)
	}
	if len(c) != 1 || c[0].Response != "answer, escaped" {
		t.Fatal(c)
	}
	put(t, root, "data/trec.qrels", "q1 0 unknown 2\n")
	if _, _, err = e.readSource(d, Plan{Source: "trec", Mode: "replay"}); err == nil {
		t.Fatal("missing association accepted")
	}
}

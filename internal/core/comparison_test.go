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

func TestComparisonFrozenInputParametersAndPortableBundle(t *testing.T) {
	e, _, root := fixture(t)
	var calls atomic.Int32
	var order []int
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["query"] != "question" || body["qrels"] != nil || body["references"] != nil {
			t.Error("frozen input changed or reference leaked")
		}
		params := body["parameters"].(map[string]any)
		k := int(params["top_k"].(float64))
		order = append(order, k)
		// Mutate the original source after preparation. Every candidate still reads
		// the common frozen copy, not this changed package.
		put(t, root, "data/input.jsonl", `{"id":"mutated","inputs":{"text":"different"}}`+"\n")
		json.NewEncoder(w).Encode(map[string]any{"answer": "ok", "retrieval": []any{map[string]any{"id": "d1", "text": "one"}}, "effective_parameters": params})
	}))
	defer target.Close()
	put(t, root, "sources/data.yaml", "id: data\nversion: '1'\nformat: jsonl\npath: data/input.jsonl\n")
	put(t, root, "data/input.jsonl", `{"id":"q","inputs":{"text":"question"},"references":{"qrels":{"d1":1}}}`+"\n")
	put(t, root, "adapters/rag.yaml", "id: rag\ntype: http\nendpoint: "+target.URL+"\nrequest:\n  query: $.text\n  parameters: $parameters\nresponse_path: $.answer\nretrieval_path: $.retrieval\napplied_parameters_path: $.effective_parameters\n")
	put(t, root, "plans/compare.yaml", "schema_version: evaluate.plan/v1\nid: compare\nmode: invoke\npreset: builtin.rag-basic@1\nsource: source:data\nsubject: adapter:rag\nparameters:\n  k: [1, 5]\ncandidates:\n  - id: base\n    target_parameters: {top_k: 1}\n  - id: expanded\n    target_parameters: {top_k: 3}\n")
	r := runPlan(t, e, "compare")
	if calls.Load() != 2 || len(order) != 2 || order[0] != 1 || order[1] != 3 || r.Comparison == nil || r.Total != 2 {
		t.Fatalf("comparison call/order: %v %+v", order, r)
	}
	for _, cr := range r.Cases {
		if cr.Result.ParameterReceipt.State != "target_confirmed" {
			t.Fatal("missing receipt")
		}
	}
	summary := r.Outputs[0].Data["comparison"].(map[string]any)
	if len(summary["rows"].([]any)) == 0 {
		t.Fatal("missing comparison statistics")
	}
	dest := filepath.Join(t.TempDir(), "comparison.seven.eval")
	if err := Export(r, dest); err != nil {
		t.Fatal(err)
	}
	imported, err := ImportFile(dest)
	if err != nil || Digest(imported) != Digest(r) {
		t.Fatalf("portable comparison: %v", err)
	}
	imported.Cases[1].Case.Inputs = map[string]any{"text": "different candidate input"}
	if ValidateRun(imported) == nil {
		t.Fatal("comparison bundle accepted different candidate inputs")
	}
	bad := r
	bad.Comparison = &Comparison{BaselineID: "other", CaseIDs: []string{"q"}, InputDigest: r.Comparison.InputDigest}
	if ValidateRun(bad) == nil {
		t.Fatal("invalid baseline accepted")
	}
}

func TestComparisonInvalidLaterCandidateMakesNoBusinessCalls(t *testing.T) {
	e, _, root := fixture(t)
	var calls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Write([]byte(`{"answer":"unexpected business call"}`))
	}))
	defer target.Close()
	put(t, root, "adapters/target.yaml", "id: target\ntype: http\nendpoint: "+target.URL+"\nrequest: {query: $, top_k: $parameters.top_k}\nresponse_path: $.answer\n")
	put(t, root, "plans/compare.yaml", "schema_version: evaluate.plan/v1\nid: compare\nmode: invoke\ndataset: dataset:sample\nsubject: adapter:target\ncandidates:\n  - id: valid-first\n    target_parameters: {top_k: 1}\n  - id: invalid-later\n    target_parameters: {strategy: ranked}\n")
	if _, err := e.Prepare(context.Background(), "compare"); err == nil {
		t.Fatal("later candidate with missing mapped parameter accepted")
	}
	if calls.Load() != 0 {
		t.Fatal("invalid comparison made business calls before rejecting the plan")
	}
}

func TestComparisonMappingPreflightAndReceipts(t *testing.T) {
	if !equalParameter(json.Number("3.0"), 3) || equalParameter(true, 1) || equalParameter(json.Number("9007199254740993"), json.Number("9007199254740992")) {
		t.Fatal("parameter JSON equality lost type or precision")
	}
	requested := map[string]any{"top_k": 3}
	for _, tc := range []struct {
		applied map[string]any
		state   string
	}{
		{nil, "unconfirmed"}, {map[string]any{"top_k": 3, "default": true}, "target_confirmed"}, {map[string]any{"top_k": 1}, "mismatch"},
	} {
		if parameterReceipt(requested, tc.applied).State != tc.state {
			t.Fatal(tc.state)
		}
	}
	if _, err := mapRequest(map[string]any{"parameter": "$parameters.missing"}, map[string]any{}, requested); err == nil {
		t.Fatal("missing target field accepted")
	}
	p := Plan{Mode: "replay", Candidates: []Candidate{{ID: "a", TargetParameters: requested}, {ID: "b", TargetParameters: requested}}}
	if validateCandidates(p, Discovery{}) == nil {
		t.Fatal("replay comparison accepted")
	}
	p.Mode = "invoke"
	p.Candidates[1].ID = "a"
	if validateCandidates(p, Discovery{}) == nil {
		t.Fatal("duplicate candidate accepted")
	}
}

func TestComparisonPythonSubjectAndProcessorOwnership(t *testing.T) {
	e, _, root := fixture(t)
	put(t, root, "adapters/python.py", `from seven_evaluate import subject
@subject(id="python",version="1")
def invoke(ctx):
    parameters=ctx["target_parameters"]
    return {"response":parameters,"parameter_receipt":{"requested":{},"applied":parameters,"state":"unconfirmed"}}
`)
	put(t, root, "processors/custom.py", `from seven_evaluate import processor
@processor(id="custom",version="1")
def process(ctx):
    assert ctx["comparison"]["baseline_id"]=="a"
    return {"processor":"team.custom@1","data":{"custom_candidates":len(ctx["candidates"])},"input_digest":ctx["input_digest"]}
`)
	base := "schema_version: evaluate.plan/v1\nid: compare\nmode: invoke\ndataset: dataset:sample\nsubject: adapter:python\ncandidates:\n  - id: a\n    target_parameters: {top_k: 1}\n  - id: b\n    target_parameters: {top_k: 2}\n"
	for _, tc := range []struct{ name, stage string }{{"default", ""}, {"disabled", "processors: []\n"}, {"custom", "processors: [processor:custom]\n"}} {
		t.Run(tc.name, func(t *testing.T) {
			put(t, root, "plans/compare.yaml", base+tc.stage)
			r := runPlan(t, e, "compare")
			if len(r.Cases) != 2 || r.Cases[0].Result.ParameterReceipt.State != "target_confirmed" {
				t.Fatal("Python parameters/receipt missing")
			}
			switch tc.name {
			case "disabled":
				if len(r.Outputs) != 0 {
					t.Fatal("disabled processor substituted")
				}
			case "custom":
				if len(r.Outputs) != 1 || r.Outputs[0].Processor != "team.custom@1" || r.Outputs[0].Data["comparison"] != nil {
					t.Fatal("custom processor mixed with default")
				}
			}
		})
	}
}

func TestComparisonCancellationRetainsCompletedCases(t *testing.T) {
	e, _, root := fixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Write([]byte(`{"answer":"ok"}`))
		cancel()
	}))
	defer target.Close()
	put(t, root, "adapters/target.yaml", "id: target\ntype: http\nendpoint: "+target.URL+"\nrequest: {query: $, parameters: $parameters}\nresponse_path: $.answer\n")
	put(t, root, "plans/compare.yaml", "schema_version: evaluate.plan/v1\nid: compare\nmode: invoke\ndataset: dataset:demo\nsubject: adapter:target\ncandidates:\n  - id: a\n    target_parameters: {top_k: 1}\n  - id: b\n    target_parameters: {top_k: 2}\n")
	// Use the fixture's actual dataset identity rather than assuming demo.
	d, err := Discover(context.Background(), root, e.Worker)
	if err != nil {
		t.Fatal(err)
	}
	var dataset string
	for _, a := range d.Assets {
		if a.Kind == "dataset" {
			dataset = a.ID
			break
		}
	}
	put(t, root, "plans/compare.yaml", "schema_version: evaluate.plan/v1\nid: compare\nmode: invoke\ndataset: dataset:"+dataset+"\nsubject: adapter:target\ncandidates:\n  - id: a\n    target_parameters: {top_k: 1}\n  - id: b\n    target_parameters: {top_k: 2}\n")
	r, err := e.Prepare(context.Background(), "compare")
	if err != nil {
		t.Fatal(err)
	}
	r, err = e.Execute(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "cancelled" || calls.Load() != 1 || len(r.Cases) != 1 {
		t.Fatalf("cancel lost partial: %s %d %d", r.Status, calls.Load(), len(r.Cases))
	}
	if err = ValidateRun(r); err != nil {
		t.Fatal(err)
	}
}

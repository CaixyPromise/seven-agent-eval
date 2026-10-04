package core

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestNodeWorkerKindsAndLossless(t *testing.T) {
	e, _, root := fixture(t)
	e.Worker.Package = root
	for _, kind := range []string{"subject", "evaluator", "processor", "conclusion"} {
		t.Run(kind, func(t *testing.T) {
			file := filepath.Join(root, "extensions", kind+".ts")
			put(t, root, "extensions/"+kind+".ts", `import {`+kind+`} from "@seven-agent/eval"; export default `+kind+`({id:"demo"},async ctx=>{console.log("not protocol");return ctx;});`)
			var descriptor map[string]any
			if err := e.Worker.Call(context.Background(), "describe", map[string]any{"path": file}, &descriptor); err != nil {
				t.Fatal(err)
			}
			if descriptor["kind"] != kind {
				t.Fatal(descriptor)
			}
			method := map[string]string{"subject": "invoke", "evaluator": "evaluate", "processor": "reduce", "conclusion": "conclude"}[kind]
			input := map[string]any{"values": []any{nil, false, 0, json.Number("9007199254740993")}}
			var got map[string]any
			if err := e.Worker.Call(context.Background(), kind+"/"+method, map[string]any{"path": file, "context": input}, &got); err != nil {
				t.Fatal(err)
			}
			if Digest(got) != Digest(input) {
				t.Fatalf("precision/type changed: %v", got)
			}
		})
	}
}

func TestNodeSnapshotAndKPI(t *testing.T) {
	e, _, root := fixture(t)
	put(t, root, "processors/_helper.ts", `export const count=7;`)
	put(t, root, "processors/team.ts", `import {processor} from "@seven-agent/eval"; import {count} from "./_helper"; export default processor({id:"team"},ctx=>({processor:"team@1",input_digest:ctx.input_digest,data:{count,coverage:0,anomalies:null},report:{version:"1",kpis:{scored:"/count",coverage:"/coverage",anomalies:"/anomalies"}}}));`)
	put(t, root, "plans/ts.yaml", "schema_version: evaluate.plan/v1\nid: ts\nmode: replay\ndataset: dataset:sample\nprocessors: [processor:team]\nconclusions: []\n")
	r, err := e.Prepare(context.Background(), "ts")
	if err != nil {
		t.Fatal(err)
	}
	put(t, root, "processors/_helper.ts", `export const count=99;`)
	r, err = executeFixture(t, e, context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Outputs) != 1 {
		t.Fatalf("missing TS output: %+v", r)
	}
	k := ResolveReportKPIs(r.Outputs)
	if k.Scored == nil || *k.Scored != 7 || k.Coverage == nil || *k.Coverage != 0 || k.Anomalies != nil {
		t.Fatalf("wrong frozen KPI %+v", k)
	}
	if r.Effective.NodeRuntime == nil {
		t.Fatal("runtime not frozen")
	}
}

func TestNodeFailureAndCancellation(t *testing.T) {
	e, _, root := fixture(t)
	e.Worker.Package = root
	for name, body := range map[string]string{
		"rejection": `throw new Error("SECRET")`, "exit": `process.exit(7)`,
		"undefined": `return undefined`, "nan": `return NaN`, "pollution": `process.stdout.write("SECRET"); return {}`,
		"oversize": `return "x".repeat(9*1024*1024)`, "timeout": `await new Promise(()=>{})`,
	} {
		t.Run(name, func(t *testing.T) {
			put(t, root, "processors/fail.ts", `import {processor} from "@seven-agent/eval"; export default processor({id:"fail"},async ()=>{`+body+`});`)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			var result any
			err := e.Worker.Call(ctx, "processor/reduce", map[string]any{"path": filepath.Join(root, "processors/fail.ts")}, &result)
			if err == nil || strings.Contains(err.Error(), "SECRET") {
				t.Fatalf("unsafe failure: %v", err)
			}
			if name == "timeout" && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal(err)
			}
		})
	}
}

func TestNodeRestrictedImportsAndUnavailable(t *testing.T) {
	e, _, root := fixture(t)
	e.Worker.Package = root
	for _, imp := range []string{`import x from "third-party";`, `import x from "../../outside.ts";`, `const x=require("node:fs");`} {
		put(t, root, "processors/bad.ts", imp+`export default x;`)
		if _, err := compileNode(root, filepath.Join(root, "processors/bad.ts"), filepath.Join(e.Worker.Home, "sdk/typescript/src/index.ts")); err == nil {
			t.Fatal("unsupported import accepted", imp)
		}
	}
	e.Worker.Node = filepath.Join(root, "missing-node")
	var got any
	if err := e.Worker.Call(context.Background(), "describe", map[string]any{"path": filepath.Join(root, "processors/bad.ts")}, &got); err == nil {
		t.Fatal("missing runtime accepted")
	}
	if _, err := os.Stat(e.Worker.Python); err != nil {
		t.Fatal(err)
	}
	put(t, root, "processors/python.py", "from seven_evaluate import processor\n@processor(id='python')\ndef process(ctx): return {}\n")
	if err := e.Worker.Call(context.Background(), "describe", map[string]any{"path": filepath.Join(root, "processors/python.py")}, &got); err != nil {
		t.Fatal("Node absence broke Python", err)
	}
}

func TestReportMappingValidationAndConflict(t *testing.T) {
	valid := ProcessorOutput{Processor: "custom@1", InputDigest: "x", Data: map[string]any{"scored": 0, "coverage": nil, "a/b": 2}, Report: &ReportMapping{Version: "1", KPIs: map[string]string{"scored": "/scored", "coverage": "/coverage", "anomalies": "/a~1b"}}}
	if err := validateProcessorOutput(valid); err != nil {
		t.Fatal(err)
	}
	k := ResolveReportKPIs([]ProcessorOutput{valid})
	if k.Scored == nil || *k.Scored != 0 || k.Coverage != nil {
		t.Fatal(k)
	}
	if !ResolveReportKPIs([]ProcessorOutput{valid, valid}).Conflict {
		t.Fatal("sources merged")
	}
	for _, path := range []string{"/missing", "bad", "/~2"} {
		bad := valid
		bad.Report = &ReportMapping{Version: "1", KPIs: map[string]string{"scored": path}}
		if validateProcessorOutput(bad) == nil {
			t.Fatal(path)
		}
	}
	for _, value := range []any{false, -1, 1.5, "0", json.Number("9007199254740993")} {
		bad := valid
		bad.Data = map[string]any{"scored": value, "coverage": nil, "a/b": 2}
		if validateProcessorOutput(bad) == nil {
			t.Fatal(value)
		}
	}
}

func TestNodeEpisodeLifecycleAndIsolation(t *testing.T) {
	e, _, root := fixture(t)
	log := filepath.Join(root, "lifecycle.log")
	put(t, root, "adapters/session.ts", `import {subject} from "@seven-agent/eval";
import {appendFileSync} from "node:fs"; import {randomUUID} from "node:crypto";
const record=x=>appendFileSync(`+strconv.Quote(log)+`,JSON.stringify(x)+"\n");
export default subject({id:"session"},ctx=>{if("references" in ctx)throw Error("reference leaked");record({action:"invoke",session:ctx.session,inputs:ctx.inputs});return {response:ctx.session.id};},
{create:()=>{const session={id:randomUUID()};record({action:"create",session});return session;},cleanup:ctx=>{record({action:"cleanup",session:ctx.session});return null;}});`)
	put(t, root, "plans/session.yaml", "schema_version: evaluate.plan/v1\nid: session\nmode: invoke\ndataset: dataset:sample\nsubject: adapter:session\nparameters:\n  cleanup_sessions: true\n")
	put(t, root, "datasets/sample/cases.jsonl", `{"id":"one","inputs":{},"episode":{"id":"one","turns":[{"id":"a","case":{"id":"a","inputs":{"turn":1},"references":{"exact":"private"}}},{"id":"b","case":{"id":"b","inputs":{"turn":2}}}]}}`+"\n"+`{"id":"two","inputs":{},"episode":{"id":"two","turns":[{"id":"a","case":{"id":"a","inputs":{"turn":1}}}]}}`+"\n")
	r := runPlan(t, e, "session")
	if r.Status != "completed" || len(r.Cases) != 3 || r.Cases[0].Result.Response != r.Cases[1].Result.Response || r.Cases[0].Result.Response == r.Cases[2].Result.Response {
		t.Fatalf("session isolation failed: %+v", r)
	}
	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	expected := []string{"create", "invoke", "invoke", "create", "invoke", "cleanup", "cleanup"}
	if len(lines) != len(expected) {
		t.Fatalf("extra calls: %s", b)
	}
	for i, line := range lines {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil || entry["action"] != expected[i] {
			t.Fatalf("bad order %s", b)
		}
	}
}

func TestNodeStreamExtensions(t *testing.T) {
	e, _, root := fixture(t)
	put(t, root, "evaluators/present.js", `import {evaluator} from "@seven-agent/eval"; export default evaluator({id:"present",metrics:["output.present"]},ctx=>[{id:"present",metric_id:"output.present",case_id:ctx.case.id,status:"observed",value:ctx.result.response!==null,evaluator:"present@1"}]);`)
	put(t, root, "processors/window.ts", `import {processor} from "@seven-agent/eval";export default processor({id:"window"},ctx=>ctx.phase==="initialize"?{count:0}:ctx.phase==="update"?{count:ctx.state.count+ctx.cases.length}:({processor:"window@1",input_digest:ctx.input_digest,data:{count:ctx.state.count,window:{scored:ctx.state.count,received:ctx.total,series:{time:ctx.window.start,scored:ctx.state.count}}}}));`)
	put(t, root, "plans/production.yaml", "schema_version: evaluate.plan/v1\nid: production\nmode: stream\nevaluators: [evaluator:present]\nprocessors: [processor:window]\nconclusions: []\nstream:\n  unit: trace\n")
	m, err := NewLiveManager(e)
	if err != nil {
		t.Fatal(err)
	}
	m.Start()
	defer m.Close()
	if _, err = m.Ingest(context.Background(), IngestBatch{PlanID: "production", Events: []ProductionEvent{liveEvent("ts", 1, "hello")}}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		report, err := m.Report(context.Background(), "production", time.Now().Add(-time.Hour), time.Now().Add(time.Minute))
		if err == nil && report.Current != nil && len(report.Current.Cases) == 1 && len(report.Current.Outputs) == 1 {
			r := report.Current
			if r.Outputs[0].Processor != "window@1" || len(r.Cases[0].Metrics) != 1 || r.Cases[0].Metrics[0].Status != "observed" {
				t.Fatalf("wrong stream result %+v", r)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("TS stream did not complete")
}

func TestCrossLanguageFixture(t *testing.T) {
	e, _, root := fixture(t)
	e.Worker.Package = root
	input, err := os.ReadFile(filepath.Join(e.Worker.Home, "protocol/conformance/cross-language-values.json"))
	if err != nil {
		t.Fatal(err)
	}
	var ctx any
	if err = decodeJSON(input, &ctx); err != nil {
		t.Fatal(err)
	}
	put(t, root, "processors/echo.ts", `import {processor} from "@seven-agent/eval";export default processor({id:"echo"},ctx=>ctx);`)
	put(t, root, "processors/echo.py", "from seven_evaluate import processor\n@processor(id='echo')\ndef echo(ctx): return ctx\n")
	for _, ext := range []string{"ts", "py"} {
		var got any
		if err = e.Worker.Call(context.Background(), "processor/reduce", map[string]any{"path": filepath.Join(root, "processors/echo."+ext), "context": ctx}, &got); err != nil {
			t.Fatal(err)
		}
		if Digest(ctx) != Digest(got) {
			t.Fatalf("%s changed shared values", ext)
		}
	}
}

func TestNodeCancellationPreservesCompletedCases(t *testing.T) {
	e, _, root := fixture(t)
	log := filepath.Join(root, "calls.log")
	put(t, root, "adapters/slow.ts", `import {subject} from "@seven-agent/eval";import {appendFileSync} from "node:fs";export default subject({id:"slow"},async ctx=>{appendFileSync(`+strconv.Quote(log)+`,ctx.inputs.id+"\n");if(ctx.inputs.id==="slow")await new Promise(()=>{});return {response:0};});`)
	put(t, root, "plans/slow.yaml", "schema_version: evaluate.plan/v1\nid: slow\nmode: invoke\ndataset: dataset:sample\nsubject: adapter:slow\n")
	put(t, root, "datasets/sample/cases.jsonl", "{\"id\":\"fast\",\"inputs\":{\"id\":\"fast\"}}\n{\"id\":\"slow\",\"inputs\":{\"id\":\"slow\"}}\n")
	r, err := e.Prepare(context.Background(), "slow")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	r, _ = executeFixture(t, e, ctx, r)
	if r.Status != "cancelled" || len(r.Cases) < 1 || r.Cases[0].Status != "completed" {
		t.Fatalf("completed case lost %+v", r)
	}
	b, err := os.ReadFile(log)
	if err != nil || string(b) != "fast\nslow\n" {
		t.Fatalf("target replayed: %s %v", b, err)
	}
}

func TestNodeComparisonReceipts(t *testing.T) {
	e, _, root := fixture(t)
	put(t, root, "adapters/target.js", `import {subject} from "@seven-agent/eval";export default subject({id:"target"},ctx=>({response:ctx.target_parameters,parameter_receipt:{requested:{},applied:ctx.target_parameters,state:"unconfirmed"}}));`)
	put(t, root, "plans/compare.yaml", "schema_version: evaluate.plan/v1\nid: compare\nmode: invoke\ndataset: dataset:sample\nsubject: adapter:target\ncandidates:\n  - id: a\n    target_parameters: {top_k: 1}\n  - id: b\n    target_parameters: {top_k: 2}\n")
	r := runPlan(t, e, "compare")
	if len(r.Cases) != 2 || r.Cases[0].Result.ParameterReceipt == nil || r.Cases[0].Result.ParameterReceipt.State != "target_confirmed" || Digest(r.Cases[0].Result.Response) == Digest(r.Cases[1].Result.Response) {
		t.Fatal("TS comparison parameters lost")
	}
}

func TestNodeDiscoveryDiagnostics(t *testing.T) {
	e, _, root := fixture(t)
	put(t, root, "processors/one.ts", `import {processor} from "@seven-agent/eval";export default processor({id:"duplicate"},()=>{throw Error("business must not run");});`)
	put(t, root, "processors/two.js", `import {processor} from "@seven-agent/eval";export default processor({id:"duplicate"},()=>({}));`)
	put(t, root, "processors/wrong.ts", `import {evaluator} from "@seven-agent/eval";export default evaluator({id:"wrong"},()=>[]);`)
	put(t, root, "processors/empty.ts", `export const helper=1;`)
	for _, name := range []string{"_helper.ts", "ignored.d.ts", "ignored.test.ts", "ignored.spec.js"} {
		put(t, root, "processors/"+name, `throw Error("must not load");`)
	}
	d, err := Discover(context.Background(), root, e.Worker)
	if err != nil {
		t.Fatal(err)
	}
	invalid := 0
	for _, a := range d.Assets {
		if strings.HasPrefix(a.Path, "processors/") {
			if a.Status != "invalid" {
				t.Fatalf("bad descriptor accepted %+v", a)
			}
			invalid++
		}
	}
	if invalid != 4 {
		t.Fatalf("ignored helper discovered: %d", invalid)
	}
}

func TestNodeJudgePolicyMissingAndErrors(t *testing.T) {
	for _, sample := range []string{"1", "0"} {
		t.Run("rate-"+sample, func(t *testing.T) {
			_, m, root := newStoppedLive(t)
			put(t, root, "evaluators/judge.ts", `import {evaluator} from "@seven-agent/eval";export default evaluator({id:"judge",execution_class:"judge",metrics:["output.present"]},ctx=>[{id:"judge",case_id:ctx.case.id,metric_id:"output.present",status:"observed",value:true,evaluator:"judge@1"}]);`)
			put(t, root, "plans/production.yaml", "schema_version: evaluate.plan/v1\nid: production\nmode: stream\nevaluators: [evaluator:basic, evaluator:judge]\nstream:\n  unit: trace\n  judge_sample_rate: "+sample+"\n  judge_per_window_budget: 1\n")
			first, second := liveEvent("a", 1, "hello"), liveEvent("b", 1, "hello")
			second.EventTime = first.EventTime
			if _, err := m.Ingest(context.Background(), IngestBatch{PlanID: "production", Events: []ProductionEvent{first, second}}); err != nil {
				t.Fatal(err)
			}
			m.scorePending(context.Background())
			two := m.records[liveKey("production", second.Source, second.EventID)]
			expected := "budget_exhausted"
			if sample == "0" {
				expected = "skipped"
			}
			found := false
			for _, metric := range two.Case.Metrics {
				if metric.Status == expected {
					found = true
					if metric.Value != nil {
						t.Fatal("policy became zero")
					}
				}
			}
			if !found {
				t.Fatalf("policy status missing %+v", two.Case.Metrics)
			}
		})
	}
	e, _, root := fixture(t)
	put(t, root, "evaluators/broken.ts", `import {evaluator} from "@seven-agent/eval";export default evaluator({id:"broken",metrics:["output.present"]},()=>{throw Error("private");});`)
	put(t, root, "plans/error.yaml", "schema_version: evaluate.plan/v1\nid: error\nmode: replay\ndataset: dataset:sample\nevaluators: [evaluator:broken]\n")
	r := runPlan(t, e, "error")
	if len(r.Cases[0].Metrics) != 1 || r.Cases[0].Metrics[0].Status != "error" || r.Cases[0].Metrics[0].Value != nil {
		t.Fatal("execution failure became score")
	}
}

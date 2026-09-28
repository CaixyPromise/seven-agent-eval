package core

import "testing"

func TestSevenHistoryDoesNotInvoke(t *testing.T) {
	e, _, root := fixture(t)
	put(t, root, "sources/history.yaml", "id: history\nversion: '1'\nformat: seven-history\npath: saved/run1\ncase: saved/case.yaml\n")
	put(t, root, "saved/case.yaml", "id: old\ninput:\n  text: original question\n")
	put(t, root, "saved/run1/run.json", `{"case_id":"old","assistant_text":"saved answer","variant_id":"workspace","duration_ms":90}`)
	put(t, root, "saved/run1/response.json", `{"assistant_text":"saved answer"}`)
	put(t, root, "saved/run1/trace.json", `{"final_status":"completed","tool_executions":[{"tool_call_id":"t1","tool_name":"search","status":"ok","tool_arguments_parsed_json":{"q":"x"},"tool_result_json":{"found":true}}]}`)
	put(t, root, "saved/run1/checks.json", `{"overall_pass":true,"checks":[{"id":"legacy","status":"passed"}]}`)
	put(t, root, "plans/history.yaml", "schema_version: evaluate.plan/v1\nid: history\nmode: replay\nsource: source:history\npreset: builtin.agent-basic@1\nsubject: adapter:missing-never-invoked\n")
	r := runPlan(t, e, "history")
	if r.Cases[0].Result.Response != "saved answer" || len(r.Cases[0].Result.Tools) != 1 || r.DatasetRevision == nil {
		t.Fatal("history normalization lost evidence")
	}
	if r.Cases[0].Case.Metadata["legacy_checks"] == nil {
		t.Fatal("checker audit lost")
	}
	tool := r.Cases[0].Result.Tools[0]
	if tool.Status != "completed" || tool.Result.(map[string]any)["found"] != true || *r.Cases[0].Result.LatencyMS != 90 {
		t.Fatal("actual runner tool/result/duration fields were not preserved")
	}
	if err := Export(r, root+"/history.seven.eval"); err != nil {
		t.Fatal(err)
	}
}

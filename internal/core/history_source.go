package core

import (
	"encoding/json"
	"errors"
	"path/filepath"
)

// A history descriptor selects one copied old runner output directory. It never
// follows artifact paths recorded by that runner and never executes old scripts.
func (e *Engine) readSevenHistory(cfg SourceConfig, rev DatasetRevision) ([]Case, DatasetRevision, error) {
	var record, trace, response, checks map[string]any
	total := 0
	for name, target := range map[string]*map[string]any{"run.json": &record, "trace.json": &trace, "response.json": &response, "checks.json": &checks} {
		rel := filepath.Join(cfg.Path, name)
		path, err := SafePath(e.Root, rel)
		if err != nil {
			return nil, rev, err
		}
		b, err := ReadBounded(path, MaxFileBytes)
		if err != nil {
			return nil, rev, err
		}
		total += len(b)
		if total > MaxFileBytes {
			return nil, rev, errors.New("history exceeds 32 MiB")
		}
		rev.Files[filepath.ToSlash(rel)] = Hash(b)
		if err = decodeJSON(b, target); err != nil {
			return nil, rev, errors.New("invalid history JSON")
		}
	}
	id, _ := record["case_id"].(string)
	if id == "" {
		return nil, rev, errors.New("history case_id missing")
	}
	answer := record["assistant_text"]
	if answer == nil {
		answer = trace["assistant_text"]
	}
	if answer == nil {
		answer = response["assistant_text"]
	}
	result := SubjectResult{Response: answer, Trace: []any{map[string]any{"legacy_trace": trace, "legacy_checks": checks}}}
	if duration, ok := record["duration_ms"].(json.Number); ok {
		if value, err := duration.Float64(); err == nil && value >= 0 {
			result.LatencyMS = &value
		}
	}
	entries, _ := trace["tool_executions"].([]any)
	for _, entry := range entries {
		tool, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		arguments, _ := tool["tool_arguments_parsed_json"].(map[string]any)
		status := "pending"
		switch tool["status"] {
		case "ok", "success", "completed":
			status = "completed"
		case "failed", "error":
			status = "error"
		}
		id, idOK := tool["tool_call_id"].(string)
		name, nameOK := tool["tool_name"].(string)
		if !idOK || id == "" || !nameOK || name == "" {
			return nil, rev, errors.New("history tool identity missing")
		}
		result.Tools = append(result.Tools, ToolEvidence{ID: id, Name: name, Arguments: arguments, Status: status, Result: tool["tool_result_json"]})
	}
	c := Case{ID: id, Inputs: map[string]any{}, Result: &result, Metadata: map[string]any{"legacy_checks": checks, "legacy_variant": record["variant_id"]}}
	if state, ok := trace["final_status"].(string); ok && state != "completed" && state != "success" {
		c.Metadata["seven_eval_subject_status"] = "error"
	}
	if cfg.CasePath != "" {
		path, err := SafePath(e.Root, cfg.CasePath)
		if err != nil {
			return nil, rev, err
		}
		b, err := ReadBounded(path, MaxFileBytes)
		if err != nil {
			return nil, rev, err
		}
		rev.Files[cfg.CasePath] = Hash(b)
		var original map[string]any
		if err = ReadDocument(path, &original); err != nil {
			return nil, rev, err
		}
		c.Inputs = original["input"]
		if c.Inputs == nil {
			c.Inputs = original["inputs"]
		}
		if c.Inputs == nil {
			c.Inputs = map[string]any{}
		}
		c.Metadata["legacy_case"] = original
	}
	rev.Digest = Digest(struct {
		Revision DatasetRevision
		Cases    []Case
	}{rev, []Case{c}})
	return []Case{c}, rev, nil
}

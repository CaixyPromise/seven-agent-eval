package core

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"seven-agent-eval/protocol"
	"strings"
	"time"
)

type Engine struct {
	Root   string
	Worker Worker
	Store  *Store
}

// PlanError identifies confirmed user-correctable validation failures.
type PlanError struct{ Err error }

func (e *PlanError) Error() string { return e.Err.Error() }
func (e *PlanError) Unwrap() error { return e.Err }
func planError(err error) error {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return &PlanError{Err: err}
}

func NewID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return time.Now().UTC().Format("20060102T150405") + "-" + hex.EncodeToString(b)
}
func (e *Engine) Prepare(ctx context.Context, planID string) (Run, error) {
	d, err := Discover(ctx, e.Root, e.Worker)
	if err != nil {
		return Run{}, err
	}
	effective, err := Resolve(e.Root, d, planID)
	if err != nil {
		return Run{}, planError(err)
	}
	cases, digest, err := e.readCases(d, effective.Plan)
	if err != nil {
		return Run{}, planError(err)
	}
	var catalogs map[string]any
	c, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err = e.Worker.Call(c, "catalogs", map[string]any{}, &catalogs); err != nil {
		return Run{}, err
	}
	// User catalog documents are preserved in the bundle alongside the built-in tables.
	for _, a := range d.Assets {
		if a.Kind == "catalog" && a.Status == "valid" {
			catalogs["local:"+a.ID] = a.Descriptor
		}
	}
	for _, reference := range effective.Evaluators {
		a, lookupErr := Lookup(d, reference, "evaluator")
		if lookupErr != nil {
			return Run{}, planError(lookupErr)
		}
		if a.Source == "local" {
			if ids, ok := a.Descriptor["metrics"].([]any); ok {
				for _, id := range ids {
					if !catalogContains(catalogs, fmt.Sprint(id)) {
						return Run{}, planError(fmt.Errorf("metric %v missing from catalogs", id))
					}
				}
			}
		}
	}
	r := Run{SchemaVersion: Version, ID: NewID(), Package: d.Package, CreatedAt: time.Now().UTC(), Status: "running",
		Total: len(cases), Effective: effective, DatasetDigest: digest, Cases: []CaseResult{}, Outputs: []ProcessorOutput{},
		Conclusions: []Conclusion{}, Catalogs: catalogs}
	if err = e.snapshot(&r); err != nil {
		return Run{}, err
	}
	if b, er := json.Marshal(e.redactForRun(&r, r.Effective)); er == nil {
		_ = decodeJSON(b, &r.Effective)
	}
	r.Effective.Digest = ""
	r.Effective.Digest = Digest(r.Effective)
	return r, e.Store.Commit(&r, "run.started", nil)
}

// PrepareStream freezes the same recipe assets as an offline run but deliberately
// has no dataset snapshot or Subject. Production events already carry evidence.
func (e *Engine) PrepareStream(ctx context.Context, planID string) (Run, error) {
	d, err := Discover(ctx, e.Root, e.Worker)
	if err != nil {
		return Run{}, err
	}
	effective, err := Resolve(e.Root, d, planID)
	if err != nil {
		return Run{}, planError(err)
	}
	if effective.Plan.Mode != "stream" {
		return Run{}, planError(errors.New("plan is not a stream"))
	}
	// Stream plans intentionally have no input snapshot. Keep the serialized
	// effective configuration schema-valid and explicit rather than emitting a
	// JSON null for its provenance map.
	effective.Files = map[string]string{}
	var catalogs map[string]any
	c, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err = e.Worker.Call(c, "catalogs", map[string]any{}, &catalogs); err != nil {
		return Run{}, err
	}
	for _, a := range d.Assets {
		if a.Kind == "catalog" && a.Status == "valid" {
			catalogs["local:"+a.ID] = a.Descriptor
		}
	}
	for _, reference := range effective.Evaluators {
		a, lookupErr := Lookup(d, reference, "evaluator")
		if lookupErr != nil {
			return Run{}, planError(lookupErr)
		}
		if a.Source == "local" {
			if ids, ok := a.Descriptor["metrics"].([]any); ok {
				for _, id := range ids {
					if !catalogContains(catalogs, fmt.Sprint(id)) {
						return Run{}, planError(fmt.Errorf("metric %v missing from catalogs", id))
					}
				}
			}
		}
	}
	r := Run{SchemaVersion: Version, ID: "stream-" + planID, Package: d.Package, CreatedAt: time.Now().UTC(), Status: "processing", Total: 0, Completed: 0, Effective: effective, DatasetDigest: "", Cases: []CaseResult{}, Outputs: []ProcessorOutput{}, Conclusions: []Conclusion{}, Catalogs: catalogs}
	r.Effective.Digest = Digest(r.Effective)
	return r, nil
}

// EvaluateExisting is the replay half of Execute exposed for persisted online
// evidence. It never crosses the Subject seam.
func (e *Engine) EvaluateExisting(ctx context.Context, r *Run, c Case, result SubjectResult) (CaseResult, error) {
	cr := CaseResult{Case: c, Status: "completed", Attempt: 0, Result: result, Evidence: []Evidence{{ID: c.ID + ":response", Kind: "response", Value: result.Response}}, Metrics: []Metric{}}
	if len(result.Contexts) > 0 {
		cr.Evidence = append(cr.Evidence, Evidence{ID: c.ID + ":contexts", Kind: "retrieval", Value: result.Contexts})
	}
	subjectStatus, subjectReason := cr.Status, cr.Reason
	if c.Metadata["seven_eval_evidence_state"] == "incomplete" {
		subjectStatus, subjectReason = "incomplete", "evidence_missing"
	}
	for _, ref := range r.Effective.Evaluators {
		decisions, _ := ctx.Value(liveDecisionKey{}).(map[string]string)
		if decisions[ref] == "interrupted" {
			cr.Status, cr.Reason = "error", "evaluation_interrupted"
			cr.Metrics = append(cr.Metrics, Metric{ID: c.ID + ":" + ref + ":interrupted", CaseID: c.ID, MetricID: "evaluation.error", Status: "error", Reason: "evaluation_interrupted", Evaluator: ref, Value: nil})
			continue
		}
		if decision := decisions[ref]; decision == "skipped" || decision == "budget_exhausted" {
			reason := "not_sampled"
			if decision == "budget_exhausted" {
				reason = "judge_budget_exhausted"
			}
			cr.Metrics = append(cr.Metrics, Metric{ID: c.ID + ":" + ref + ":policy", CaseID: c.ID, MetricID: "evaluation.policy", Status: decision, Reason: reason, Evaluator: ref, Value: nil})
			continue
		}
		var ms []Metric
		extensionContext := map[string]any{"case": c, "result": result, "status": subjectStatus, "reason": subjectReason}
		if identity := ctx.Value(liveIdentityKey{}); identity != nil {
			for _, asset := range r.Effective.Assets {
				if "evaluator:"+asset.ID == ref {
					extensionContext["idempotency_key"] = Digest([]any{identity, ref, asset.Version, asset.Descriptor})
				}
			}
		}
		err := e.extension(ctx, r, ref, "evaluator", "evaluator/evaluate", extensionContext, &ms)
		if err == nil {
			for _, m := range ms {
				if err = protocol.Validate("metric", m); err != nil {
					break
				}
				if m.CaseID != c.ID || !catalogContains(r.Catalogs, m.MetricID) {
					err = errors.New("metric identity/catalog mismatch")
					break
				}
			}
		}
		if err != nil {
			cr.Status, cr.Reason = "error", "evaluator_error"
			ms = []Metric{{ID: c.ID + ":" + ref + ":error", CaseID: c.ID, MetricID: "evaluation.error", Status: "error", Reason: "evaluator_error", Evaluator: ref, Value: nil}}
		}
		cr.Metrics = append(cr.Metrics, ms...)
	}
	before := Digest(cr)
	if b, err := json.Marshal(e.redactForRun(r, cr)); err == nil {
		var redacted CaseResult
		if decodeJSON(b, &redacted) == nil {
			cr = redacted
		}
	}
	cr.RedactionApplied = before != Digest(cr)
	return cr, nil
}

// ProcessExisting is shared by finite windows and ordinary runs so a closed
// window has exactly the same processor/conclusion semantics as its export.
func (e *Engine) ProcessExisting(ctx context.Context, r *Run) error {
	r.Outputs, r.Conclusions = []ProcessorOutput{}, []Conclusion{}
	inputDigest := Digest(r.Cases)
	if r.Scope != nil {
		inputDigest = Digest(map[string]any{"cases": r.Cases, "start": r.Scope.Start, "end": r.Scope.End, "facts": r.Scope.Facts, "backlog_facts": r.Scope.BacklogFacts})
	}
	for _, ref := range r.Effective.Processors {
		var output ProcessorOutput
		err := e.extension(ctx, r, ref, "processor", "processor/process", map[string]any{"cases": r.Cases, "total": r.Total, "input_digest": inputDigest, "window": r.Scope}, &output)
		if err == nil {
			err = protocol.Validate("processor_output", output)
		}
		if err != nil {
			r.Message = "Processor failed; no fallback statistics were substituted"
			r.Status = "partial"
			continue
		}
		output.InputDigest = inputDigest
		r.Outputs = append(r.Outputs, output)
	}
	for _, ref := range r.Effective.Conclusions {
		var conclusions []Conclusion
		err := e.extension(ctx, r, ref, "conclusion", "conclusion/conclude", map[string]any{"outputs": r.Outputs, "cases": r.Cases}, &conclusions)
		if err == nil {
			for _, c := range conclusions {
				if err = protocol.Validate("conclusion", c); err != nil {
					break
				}
			}
		}
		if err != nil {
			r.Message = "Conclusion failed; see available facts"
			r.Status = "partial"
			continue
		}
		r.Conclusions = append(r.Conclusions, conclusions...)
	}
	return nil
}
func catalogContains(value any, id string) bool {
	switch v := value.(type) {
	case map[string]any:
		if v["id"] == id || v["metric_id"] == id {
			return true
		}
		for _, x := range v {
			if catalogContains(x, id) {
				return true
			}
		}
	case []any:
		for _, x := range v {
			if catalogContains(x, id) {
				return true
			}
		}
	}
	return false
}
func (e *Engine) readCases(d Discovery, p Plan) ([]Case, string, error) {
	a, err := Lookup(d, p.Dataset, "dataset")
	if err != nil {
		return nil, "", err
	}
	rel := a.Path
	if filepath.Base(rel) == "dataset.yaml" {
		source, _ := a.Descriptor["source"].(string)
		rel = filepath.Join(filepath.Dir(rel), source)
	}
	path, err := SafePath(e.Root, rel)
	if err != nil {
		return nil, "", err
	}
	b, err := ReadBounded(path, MaxFileBytes)
	if err != nil {
		return nil, "", err
	}
	scanner := bufio.NewScanner(bytes.NewReader(b))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	cases := []Case{}
	seen := map[string]bool{}
	line := 0
	for scanner.Scan() {
		line++
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			continue
		}
		var raw map[string]any
		if err = decodeJSON(scanner.Bytes(), &raw); err != nil {
			return nil, "", fmt.Errorf("dataset line %d: invalid JSON", line)
		}
		if err = protocol.Validate("case", raw); err != nil {
			return nil, "", fmt.Errorf("dataset line %d: %w", line, err)
		}
		if p.Mode == "replay" {
			if _, ok := raw["response"]; !ok {
				return nil, "", fmt.Errorf("dataset line %d: replay requires response field", line)
			}
		}
		var c Case
		if err = decodeJSON(scanner.Bytes(), &c); err != nil {
			return nil, "", err
		}
		if seen[c.ID] {
			return nil, "", fmt.Errorf("duplicate case id on line %d", line)
		}
		seen[c.ID] = true
		cases = append(cases, c)
		if len(cases) > MaxCases {
			return nil, "", errors.New("phase 1 limit: 1000 cases")
		}
	}
	if err = scanner.Err(); err != nil {
		return nil, "", err
	}
	if len(cases) == 0 {
		return nil, "", errors.New("empty dataset")
	}
	return cases, Hash(b), nil
}
func (e *Engine) asset(r *Run, reference, kind string) (Asset, error) {
	d := Discovery{Assets: r.Effective.Assets}
	a, err := Lookup(d, reference, kind)
	if err != nil {
		return a, err
	}
	if a.Source == "local" {
		p, err := SafePath(e.Root, a.Path)
		if err != nil {
			return a, err
		}
		b, err := ReadBounded(p, MaxFileBytes)
		if err != nil {
			return a, err
		}
		if a.Descriptor["digest"] != Hash(b) {
			return a, errors.New("asset changed after plan preparation")
		}
	}
	return a, nil
}
func (e *Engine) extension(ctx context.Context, r *Run, ref, kind, method string, input any, out any) error {
	a, err := e.asset(r, ref, kind)
	if err != nil {
		return err
	}
	params := map[string]any{"context": input}
	if a.Source == "local" {
		p, err := SafePath(e.Root, a.Path)
		if err != nil {
			return err
		}
		params["path"] = p
	}
	c, cancel := context.WithTimeout(ctx, time.Duration(r.Effective.Plan.TimeoutSeconds)*time.Second)
	defer cancel()
	return e.Worker.Call(c, method, params, out)
}
func (e *Engine) Execute(ctx context.Context, r Run) (Run, error) {
	// Execution reads only its frozen package/SDK copy, including helper modules.
	if len(r.Effective.Files) > 0 {
		copyEngine := *e
		copyEngine.Root = filepath.Join(e.Store.Root, "runs", r.ID, "source")
		copyEngine.Worker.Source = filepath.Join(e.Store.Root, "runs", r.ID, "runtime")
		e = &copyEngine
	}
	d := Discovery{Package: r.Package, Assets: r.Effective.Assets}
	cases, digest, err := e.readCases(d, r.Effective.Plan)
	if err != nil || digest != r.DatasetDigest {
		r.Status = "failed"
		r.Message = "Dataset changed or unavailable after preparation"
		save := e.Store.Commit(&r, "run.finished", nil)
		if save != nil {
			return r, save
		}
		return r, errors.New(r.Message)
	}
	for _, c := range cases {
		if ctx.Err() != nil {
			break
		}
		cr := CaseResult{Case: c, Status: "completed", Attempt: 0, Evidence: []Evidence{}, Metrics: []Metric{}}
		op, stop := context.WithTimeout(ctx, time.Duration(r.Effective.Plan.TimeoutSeconds)*time.Second)
		if r.Effective.Plan.Mode == "replay" {
			cr.Result = SubjectResult{Response: c.Response, Contexts: c.Contexts}
		} else {
			cr.Result, err = e.invoke(op, &r, c)
		}
		stop()
		if err != nil {
			cr.Status = "error"
			cr.Reason = "subject_error"
			if ctx.Err() != nil {
				cr.Status = "cancelled"
				cr.Reason = "cancelled"
			}
		}
		// Evaluate original in-memory evidence. Only persistence/display copies are redacted.
		subjectStatus, subjectReason := cr.Status, cr.Reason
		cr.Evidence = append(cr.Evidence, Evidence{ID: c.ID + ":response", Kind: "response", Value: cr.Result.Response})
		if len(cr.Result.Contexts) > 0 {
			cr.Evidence = append(cr.Evidence, Evidence{ID: c.ID + ":contexts", Kind: "retrieval", Value: cr.Result.Contexts})
		}
		for _, ref := range r.Effective.Evaluators {
			if ctx.Err() != nil {
				break
			}
			var ms []Metric
			input := map[string]any{"case": c, "result": cr.Result, "status": subjectStatus, "reason": subjectReason}
			er := e.extension(ctx, &r, ref, "evaluator", "evaluator/evaluate", input, &ms)
			if er == nil {
				for _, m := range ms {
					if er = protocol.Validate("metric", m); er != nil {
						break
					}
					if m.CaseID != c.ID || !catalogContains(r.Catalogs, m.MetricID) {
						er = errors.New("metric identity/catalog mismatch")
						break
					}
				}
			}
			if er != nil {
				cr.Status = "error"
				cr.Reason = "evaluator_error"
				ms = []Metric{{ID: c.ID + ":" + ref + ":error", CaseID: c.ID, MetricID: "evaluation.error", Status: "error", Reason: "evaluator_error", Evaluator: ref, Value: nil}}
			}
			cr.Metrics = append(cr.Metrics, ms...)
		}
		// The stored case may include references; it never crosses the subject seam.
		before := Digest(cr)
		if b, er := json.Marshal(e.redactForRun(&r, cr)); er == nil {
			_ = decodeJSON(b, &cr)
		}
		cr.RedactionApplied = before != Digest(cr)
		r.Cases = append(r.Cases, cr)
		r.Completed = len(r.Cases)
		if err = e.Store.Commit(&r, "case.completed", cr); err != nil {
			return r, err
		}
	}
	cancelled := ctx.Err() != nil
	r.Status = "processing"
	if err = e.Store.Commit(&r, "run.processing", nil); err != nil {
		return r, err
	}
	// Normal operations retain their own plan deadlines; cancelled runs have a bounded grace period.
	finalCtx := ctx
	cancel := func() {}
	if cancelled {
		finalCtx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	}
	defer func() { cancel() }()
	ensureGrace := func() {
		if !cancelled && ctx.Err() != nil {
			cancelled = true
			finalCtx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		}
	}
	inputDigest := Digest(r.Cases)
	for _, ref := range r.Effective.Processors {
		ensureGrace()
		var output ProcessorOutput
		input := map[string]any{"cases": r.Cases, "total": r.Total, "input_digest": inputDigest}
		er := e.extension(finalCtx, &r, ref, "processor", "processor/process", input, &output)
		if er == nil {
			er = protocol.Validate("processor_output", output)
		}
		if er != nil {
			r.Message = "Processor failed; no fallback statistics were substituted"
			r.Status = "partial"
			continue
		}
		output.InputDigest = inputDigest
		r.Outputs = append(r.Outputs, output)
	}
	for _, ref := range r.Effective.Conclusions {
		ensureGrace()
		var conclusions []Conclusion
		er := e.extension(finalCtx, &r, ref, "conclusion", "conclusion/conclude", map[string]any{"outputs": r.Outputs, "cases": r.Cases}, &conclusions)
		if er == nil {
			for _, c := range conclusions {
				if er = protocol.Validate("conclusion", c); er != nil {
					break
				}
			}
		}
		if er != nil {
			r.Message = "Conclusion failed; see available facts"
			r.Status = "partial"
			continue
		}
		r.Conclusions = append(r.Conclusions, conclusions...)
	}
	if cancelled || ctx.Err() != nil {
		r.Status = "cancelled"
		r.Message = "Cancelled; completed cases retained, remaining cases not scored"
	} else if r.Status != "partial" {
		r.Status = "completed"
		for _, c := range r.Cases {
			if c.Status != "completed" {
				r.Status = "partial"
				break
			}
		}
	}
	if b, er := json.Marshal(e.redactForRun(&r, r)); er == nil {
		_ = decodeJSON(b, &r)
	}
	if err = ValidateRun(r); err != nil {
		return r, err
	}
	return r, e.Store.Commit(&r, "run.finished", nil)
}
func (e *Engine) invoke(ctx context.Context, r *Run, c Case) (SubjectResult, error) {
	var result SubjectResult
	a, err := e.asset(r, r.Effective.Plan.Subject, "adapter")
	if err != nil {
		return result, err
	}
	if filepath.Ext(a.Path) == ".py" {
		path, err := SafePath(e.Root, a.Path)
		if err != nil {
			return result, err
		}
		err = e.Worker.Call(ctx, "subject/invoke", map[string]any{"path": path, "context": map[string]any{"inputs": c.Inputs}}, &result)
		if err == nil {
			err = protocol.Validate("subject_result", result)
		}
		return result, err
	}
	var cfg HTTPConfig
	p, err := SafePath(e.Root, a.Path)
	if err != nil {
		return result, err
	}
	if err = ReadDocument(p, &cfg); err != nil {
		return result, err
	}
	u, err := url.Parse(cfg.Endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
		return result, errors.New("invalid HTTP endpoint")
	}
	payload := c.Inputs
	if cfg.Request != nil {
		payload, err = mapValue(cfg.Request, c.Inputs)
		if err != nil {
			return result, err
		}
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return result, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.Endpoint, bytes.NewReader(b))
	if err != nil {
		return result, err
	}
	req.Header.Set("Content-Type", "application/json")
	secret := ""
	if cfg.AuthEnv != "" {
		secret = os.Getenv(cfg.AuthEnv)
		if secret == "" {
			return result, errors.New("auth environment variable missing")
		}
		req.Header.Set("Authorization", "Bearer "+secret)
	}
	client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	start := time.Now()
	response, err := client.Do(req)
	if err != nil {
		return result, errors.New("HTTP invocation failed")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
	if err != nil {
		return result, err
	}
	if len(body) > 2<<20 {
		return result, errors.New("HTTP output too large")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return result, fmt.Errorf("HTTP status %d", response.StatusCode)
	}
	var raw any
	if err = decodeJSON(body, &raw); err != nil {
		return result, errors.New("HTTP result must be JSON")
	}
	if cfg.ResponsePath == "" {
		result.Response = raw
	} else {
		result.Response, err = selectValue(raw, cfg.ResponsePath)
		if err != nil {
			return result, err
		}
	}
	if cfg.ContextsPath != "" {
		v, er := selectValue(raw, cfg.ContextsPath)
		if er != nil {
			return result, er
		}
		var ok bool
		result.Contexts, ok = v.([]any)
		if !ok {
			return result, errors.New("contexts must be array")
		}
	}
	if cfg.UsagePath != "" {
		v, er := selectValue(raw, cfg.UsagePath)
		if er != nil {
			return result, er
		}
		var ok bool
		result.Usage, ok = v.(map[string]any)
		if !ok {
			return result, errors.New("usage must be object")
		}
	}
	latency := float64(time.Since(start).Microseconds()) / 1000
	result.LatencyMS = &latency
	return result, protocol.Validate("subject_result", result)
}
func selectValue(input any, path string) (any, error) {
	if path == "$" {
		return input, nil
	}
	if !strings.HasPrefix(path, "$.") {
		return nil, errors.New("mapping must be $ or $.field")
	}
	value := input
	for _, key := range strings.Split(path[2:], ".") {
		m, ok := value.(map[string]any)
		if !ok {
			return nil, errors.New("mapping field parent is not object")
		}
		var found bool
		value, found = m[key]
		if !found {
			return nil, fmt.Errorf("mapping field missing: %s", key)
		}
	}
	return value, nil
}
func mapValue(template any, input any) (any, error) {
	switch v := template.(type) {
	case string:
		if strings.HasPrefix(v, "$") {
			return selectValue(input, v)
		}
		return v, nil
	case map[string]any:
		m := map[string]any{}
		for k, x := range v {
			y, e := mapValue(x, input)
			if e != nil {
				return nil, e
			}
			m[k] = y
		}
		return m, nil
	case []any:
		a := []any{}
		for _, x := range v {
			y, e := mapValue(x, input)
			if e != nil {
				return nil, e
			}
			a = append(a, y)
		}
		return a, nil
	default:
		return template, nil
	}
}
func redact(v any) any {
	b, _ := json.Marshal(v)
	var raw any
	_ = decodeJSON(b, &raw)
	var walk func(any) any
	walk = func(x any) any {
		switch t := x.(type) {
		case map[string]any:
			for k, v := range t {
				lower := strings.ToLower(k)
				if lower == "authorization" || lower == "cookie" || lower == "api_key" || lower == "password" || lower == "access_token" || lower == "secret" {
					t[k] = "[REDACTED]"
				} else {
					t[k] = walk(v)
				}
			}
		case []any:
			for i, v := range t {
				t[i] = walk(v)
			}
		}
		return x
	}
	return walk(raw)
}

package core

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"
)

// Persist the assembled span set, including both clocks, before acknowledging
// transport. Late spans advance revision and never repeat a production request.
type otlpAssembly struct {
	ExpiredSpans map[string]string `json:"expired_spans,omitempty"`
	Key          string            `json:"key"`
	PlanID       string            `json:"plan_id"`
	Resource     []otlpAttr        `json:"resource"`
	Spans        []otlpSpan        `json:"spans"`
	FirstAt      time.Time         `json:"first_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
	Revision     int               `json:"revision"`
}

func (m *LiveManager) IngestOTLP(ctx context.Context, body []byte) (IngestResult, error) {
	var req otlpRequest
	if err := decodeJSON(body, &req); err != nil {
		return IngestResult{}, errors.New("OTLP plan mapping missing")
	}
	result := IngestResult{Items: []IngestItem{}}
	count := 0
	for _, rs := range req.ResourceSpans {
		for _, ss := range rs.ScopeSpans {
			count += len(ss.Spans)
		}
	}
	if count == 0 || count > MaxLiveBatch {
		return result, errors.New("ingest batch too large")
	}
	for _, rs := range req.ResourceSpans {
		resource := attrs(rs.Resource.Attributes)
		id := resource["seven.eval.plan.id"]
		plan, err := m.plan(ctx, id)
		if err != nil {
			return result, err
		}
		if plan.Effective.Plan.Stream.Unit == "turn" && resource["seven.eval.turn.id"] == "" {
			return result, errors.New("OTLP turn mapping missing")
		}
		groups := map[string][]otlpSpan{}
		order := []string{}
		for _, ss := range rs.ScopeSpans {
			for _, span := range ss.Spans {
				if _, ok := groups[span.TraceID]; !ok {
					order = append(order, span.TraceID)
				}
				groups[span.TraceID] = append(groups[span.TraceID], span)
			}
		}
		for _, trace := range order {
			incoming := groups[trace]
			if trace == "" {
				return result, errors.New("OTLP root mapping missing")
			}
			key := Digest([]string{id, resource["service.name"], resource["seven.eval.project"], trace, resource["seven.eval.turn.id"]})
			m.mu.Lock()
			assembly, exists := m.assemblies[key]
			if assembly.ExpiredSpans != nil {
				code := ""
				for _, span := range incoming {
					hash, found := assembly.ExpiredSpans[span.SpanID]
					if !found {
						code = "DATA_EXPIRED"
						break
					}
					if hash != Digest(span) {
						code = "EVENT_CONFLICT"
						break
					}
				}
				if code == "" {
					result.Duplicate++
					result.Items = append(result.Items, IngestItem{EventID: trace, Status: "duplicate"})
				} else {
					result.Rejected++
					result.Items = append(result.Items, IngestItem{EventID: trace, Status: "rejected", Code: code})
				}
				m.mu.Unlock()
				continue
			}
			expired := false
			for _, span := range incoming {
				if span.EndTimeUnixNano > 0 && m.expiredEventLocked(id, time.Unix(0, int64(span.EndTimeUnixNano))) {
					expired = true
				}
			}
			if expired {
				result.Rejected++
				result.Items = append(result.Items, IngestItem{EventID: trace, Status: "rejected", Code: "DATA_EXPIRED"})
				m.mu.Unlock()
				continue
			}
			if !exists {
				pending := 0
				for _, a := range m.assemblies {
					if a.PlanID == id && !m.assemblyEmittedLocked(a) {
						pending++
					}
				}
				if pending >= plan.Effective.Plan.Stream.MaxPending {
					m.mu.Unlock()
					return result, errors.New("ingest queue full")
				}
				assembly = otlpAssembly{Key: key, PlanID: id, Resource: rs.Resource.Attributes, FirstAt: time.Now().UTC()}
			}
			spans := map[string]otlpSpan{}
			for _, span := range assembly.Spans {
				spans[span.SpanID] = span
			}
			changed, conflict := false, false
			for _, span := range incoming {
				if span.SpanID == "" {
					conflict = true
					break
				}
				if previous, ok := spans[span.SpanID]; ok {
					if Digest(previous) != Digest(span) {
						conflict = true
						break
					}
				} else {
					spans[span.SpanID] = span
					changed = true
				}
			}
			if conflict {
				result.Rejected++
				result.Items = append(result.Items, IngestItem{EventID: trace, Status: "rejected", Code: "EVENT_CONFLICT"})
				m.mu.Unlock()
				continue
			}
			if !changed {
				result.Duplicate++
				result.Items = append(result.Items, IngestItem{EventID: trace, Status: "duplicate"})
				m.mu.Unlock()
				continue
			}
			assembly.Spans = nil
			for _, span := range spans {
				assembly.Spans = append(assembly.Spans, span)
			}
			sort.Slice(assembly.Spans, func(i, j int) bool { return assembly.Spans[i].SpanID < assembly.Spans[j].SpanID })
			assembly.UpdatedAt = time.Now().UTC()
			assembly.Revision++
			encoded, _ := json.Marshal(assembly)
			if len(encoded) > MaxLiveEventBytes {
				m.mu.Unlock()
				result.Rejected++
				result.Items = append(result.Items, IngestItem{EventID: trace, Status: "rejected", Code: "EVENT_TOO_LARGE"})
				continue
			}
			err = m.appendLocked(liveJournal{Type: "otlp.updated", PlanID: id, Assembly: &assembly})
			if err == nil {
				m.assemblies[key] = assembly
				result.Accepted++
				result.Items = append(result.Items, IngestItem{EventID: trace, Status: "accepted"})
			}
			m.mu.Unlock()
			if err != nil {
				return result, err
			}
		}
	}
	m.signal()
	return result, nil
}

func (m *LiveManager) flushOTLP(ctx context.Context) {
	m.mu.Lock()
	assemblies := []otlpAssembly{}
	for _, a := range m.assemblies {
		if !m.assemblyEmittedLocked(a) {
			assemblies = append(assemblies, a)
		}
	}
	m.mu.Unlock()
	sort.Slice(assemblies, func(i, j int) bool { return assemblies[i].FirstAt.Before(assemblies[j].FirstAt) })
	for _, a := range assemblies {
		if ctx.Err() != nil {
			return
		}
		plan, err := m.plan(ctx, a.PlanID)
		if err != nil {
			continue
		}
		cfg := plan.Effective.Plan.Stream
		rs := otlpResourceSpan{}
		rs.Resource.Attributes = a.Resource
		rs.ScopeSpans = append(rs.ScopeSpans, struct {
			Spans []otlpSpan `json:"spans"`
		}{Spans: a.Spans})
		body, _ := json.Marshal(otlpRequest{ResourceSpans: []otlpResourceSpan{rs}})
		batch, mapErr := MapOTLPJSON(body)
		complete := mapErr == nil && len(batch.Events) > 0 && batch.Events[0].Result.Response != nil && batch.Events[0].Result.Response != ""
		if complete {
			if time.Since(a.UpdatedAt) < time.Duration(cfg.QuietSeconds)*time.Second {
				continue
			}
		} else if time.Since(a.FirstAt) < time.Duration(cfg.EvidenceTimeoutSeconds)*time.Second {
			continue
		}
		if mapErr != nil {
			resource := attrs(a.Resource)
			source := resource["service.name"]
			if source == "" {
				source = "otlp"
			}
			project := resource["seven.eval.project"]
			if project == "" {
				project = source
			}
			batch = IngestBatch{PlanID: a.PlanID, Events: []ProductionEvent{{SchemaVersion: "evaluate.event/v1", Source: source, Project: project, TraceID: a.Spans[0].TraceID, TurnID: resource["seven.eval.turn.id"], EventTime: a.FirstAt, Case: Case{ID: a.Key, Inputs: nil}, Result: SubjectResult{Response: nil}}}}
		}
		for i := range batch.Events {
			event := &batch.Events[i]
			event.EventID = "otlp:" + a.Key + ":" + Digest(a.Spans)
			event.EvidenceRevision = a.Revision
			if !complete {
				event.Result.Response = nil
				event.Case.Metadata = map[string]any{"seven_eval_evidence_state": "incomplete"}
			}
		}
		_, _ = m.Ingest(ctx, batch)
	}
}

func (m *LiveManager) assemblyEmittedLocked(a otlpAssembly) bool {
	if a.ExpiredSpans != nil {
		return true
	}
	source := attrs(a.Resource)["service.name"]
	if source == "" {
		source = "otlp"
	}
	return m.records[liveKey(a.PlanID, source, "otlp:"+a.Key+":"+Digest(a.Spans))] != nil
}

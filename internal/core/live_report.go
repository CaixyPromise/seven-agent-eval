package core

import (
	"context"
	"encoding/json"
	"time"
)

type BacklogPoint struct {
	Time    time.Time `json:"time"`
	Pending int       `json:"pending"`
}

// A range query uses the same Processor with the union of latest window facts.
// Only the Processor calculates metrics or samples the observation-time backlog.
func (m *LiveManager) rangeSummary(ctx context.Context, base Run, report *LiveReport, snapshot string) error {
	ids := make([]string, 0, len(report.Windows))
	for _, w := range report.Windows {
		ids = append(ids, w.ID)
	}
	var count, sequence, scoredSequence int
	m.mu.Lock()
	err := m.db.QueryRow("SELECT count(*),coalesce(max(seq),0),coalesce(max(scored_seq),0) FROM live_snapshots WHERE id=? AND window_ns>=? AND window_ns<?", snapshot, report.From.UnixNano(), report.To.UnixNano()).Scan(&count, &sequence, &scoredSequence)
	m.mu.Unlock()
	if err != nil {
		return err
	}
	key := Digest([]any{base.Effective, ids, count, sequence, scoredSequence})
	if key == m.rangeCacheKey {
		report.Summary = cloneScope(m.rangeCacheSummary)
		if report.Summary != nil {
			report.Summary.Start, report.Summary.End, report.Summary.Watermark = report.From, report.To, report.To
		}
		report.BacklogSeries = append([]BacklogPoint(nil), m.rangeCacheBacklog...)
		return nil
	}
	r := cloneLiveRun(base)
	r.Scope = &WindowScope{Kind: "window", PlanID: report.Plan.ID, Start: report.From, End: report.To, Watermark: report.To, Revision: 1, State: "provisional"}
	r.Total = 0
	r.Completed = 0
	r.Cases = nil
	for _, w := range report.Windows {
		full, err := m.Engine.Store.Get(w.ID)
		if err != nil {
			return err
		}
		r.Total += full.Total
		r.Completed += full.Completed
		r.Scope.Facts = append(r.Scope.Facts, full.Scope.Facts...)
		if len(r.Scope.Facts) > MaxLiveRangeRecords {
			return ErrLiveCapacity
		}
	}
	r.caseSource = func(visit func(CaseResult) error) error {
		for _, w := range report.Windows {
			full, err := m.Engine.Store.Get(w.ID)
			if err != nil {
				return err
			}
			if err = full.ForEachCase(visit); err != nil {
				return err
			}
		}
		return nil
	}
	err = m.walkSnapshot(snapshot, func(record liveRecord) error {
		if record.WindowTime.Before(report.From) || !record.WindowTime.Before(report.To) {
			return nil
		}
		r.Scope.BacklogFacts = append(r.Scope.BacklogFacts, WindowFact{ReceivedAt: record.ReceivedAt, ScoredAt: record.ScoredAt})
		return nil
	})
	if err != nil {
		return err
	}
	b, err := json.Marshal(r.Scope)
	if err != nil {
		return err
	}
	if len(b) > MaxMetadataBytes {
		return ErrLiveCapacity
	}
	if err = m.Engine.ProcessExisting(ctx, &r); err != nil {
		return err
	}
	for _, output := range r.Outputs {
		if data, ok := output.Data["window"].(map[string]any); ok {
			body, _ := json.Marshal(data)
			var summary WindowScope
			if decodeJSON(body, &summary) == nil {
				summary.Kind, summary.PlanID, summary.State = "window", report.Plan.ID, "provisional"
				summary.Start, summary.End, summary.Watermark = report.From, report.To, report.To
				summary.Revision = 1
				for _, window := range report.Windows {
					if window.Scope.Revision > summary.Revision {
						summary.Revision = window.Scope.Revision
					}
				}
				summary.StatisticsAvailable = true
				report.Summary = &summary
			}
			body, _ = json.Marshal(data["backlog_series"])
			_ = decodeJSON(body, &report.BacklogSeries)
		}
	}
	m.rangeCacheKey = key
	m.rangeCacheSummary = cloneScope(report.Summary)
	m.rangeCacheBacklog = append([]BacklogPoint(nil), report.BacklogSeries...)
	return nil
}

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
func (m *LiveManager) rangeSummary(ctx context.Context, base Run, report *LiveReport) {
	r := cloneLiveRun(base)
	r.Scope = &WindowScope{Kind: "window", PlanID: report.Plan.ID, Start: report.From, End: report.To, Watermark: report.To, Revision: 1, State: "provisional"}
	r.Cases = []CaseResult{}
	for _, window := range report.Windows {
		r.Cases = append(r.Cases, window.Cases...)
		r.Scope.Facts = append(r.Scope.Facts, window.Scope.Facts...)
		r.Total += window.Total
	}
	r.Completed = len(r.Cases)
	m.mu.Lock()
	for _, record := range m.records {
		if record.PlanID == report.Plan.ID && !record.WindowTime.Before(report.From) && record.WindowTime.Before(report.To) {
			r.Scope.BacklogFacts = append(r.Scope.BacklogFacts, WindowFact{ReceivedAt: record.ReceivedAt, ScoredAt: record.ScoredAt})
		}
	}
	m.mu.Unlock()
	_ = m.Engine.ProcessExisting(ctx, &r)
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
}

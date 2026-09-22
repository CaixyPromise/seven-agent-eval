package core

import (
	"strconv"
	"time"
)

type liveDecisionKey struct{}
type liveIdentityKey struct{}

// Decisions are persisted before any evaluator starts. Recovery reuses the
// reservation, so an interrupted evaluation cannot acquire a second budget slot.
func (m *LiveManager) judgeDecisionsLocked(plan Run, record *liveRecord) map[string]string {
	decisions := map[string]string{}
	cfg := plan.Effective.Plan.Stream
	unit, _ := unitKey(plan.Effective.Plan, record.Event)
	start := record.WindowTime.Truncate(time.Duration(cfg.WindowSeconds) * time.Second)
	for _, ref := range plan.Effective.Evaluators {
		var asset Asset
		for _, a := range plan.Effective.Assets {
			if "evaluator:"+a.ID == ref {
				asset = a
				break
			}
		}
		if asset.Descriptor["execution_class"] != "judge" {
			continue
		}
		identity := Digest([]any{unit, record.Event.EvidenceRevision, asset.Version, asset.Descriptor})
		n, _ := strconv.ParseUint(identity[:13], 16, 64)
		if float64(n)/float64(uint64(1)<<52) >= *cfg.JudgeSampleRate {
			decisions[ref] = "skipped"
			continue
		}
		used := 0
		for _, prior := range m.records {
			if prior.PlanID == record.PlanID && prior.WindowTime.Truncate(time.Duration(cfg.WindowSeconds)*time.Second).Equal(start) {
				for _, decision := range prior.Decisions {
					if decision == "selected" || decision == "interrupted" {
						used++
					}
				}
			}
		}
		for _, decision := range decisions {
			if decision == "selected" {
				used++
			}
		}
		if used >= *cfg.JudgePerWindowBudget {
			decisions[ref] = "budget_exhausted"
		} else {
			decisions[ref] = "selected"
		}
	}
	return decisions
}

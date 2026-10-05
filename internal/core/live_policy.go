package core

import (
	"strconv"
)

type liveDecisionKey struct{}
type liveIdentityKey struct{}

// Decisions are persisted before any evaluator starts. Recovery reuses the
// reservation, so an interrupted evaluation cannot acquire a second budget slot.
func (m *LiveManager) judgeDecisionsLocked(plan Run, record *liveRecord) (map[string]string, error) {
	decisions := map[string]string{}
	if err := m.ensureProjectionLocked(); err != nil {
		return nil, err
	}
	record.DecisionIdentities = map[string]string{}
	cfg := plan.Effective.Plan.Stream
	unit, _ := unitKey(plan.Effective.Plan, record.Event)

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
		budgetIdentity := Digest([]any{ref, asset.Version, asset.Descriptor, plan.Effective.Plan.Parameters})
		record.DecisionIdentities[ref] = budgetIdentity
		used := 0
		if err := m.db.QueryRow("SELECT count(*) FROM live_decisions WHERE plan=? AND start_ns=? AND identity=? AND decision IN ('selected','interrupted')", record.PlanID, record.WindowStart.UnixNano(), budgetIdentity).Scan(&used); err != nil {
			return nil, err
		}
		if used >= *cfg.JudgePerWindowBudget {
			decisions[ref] = "budget_exhausted"
		} else {
			decisions[ref] = "selected"
		}
	}
	return decisions, nil
}

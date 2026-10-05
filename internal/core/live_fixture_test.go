package core

import (
	"context"
	"testing"
	"time"
)

// Small fixture materialization is confined to tests; production uses indexes.
func testLiveRecords(t *testing.T, m *LiveManager) map[string]*liveRecord {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]*liveRecord{}
	if err := m.walkRecordsLocked(func(k string, r *liveRecord) error { copy := *r; out[k] = &copy; return nil }); err != nil {
		t.Fatal(err)
	}
	return out
}
func testLiveAssemblies(t *testing.T, m *LiveManager) map[string]otlpAssembly {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]otlpAssembly{}
	if err := m.walkAssembliesLocked(func(k string, a otlpAssembly) error { out[k] = a; return nil }); err != nil {
		t.Fatal(err)
	}
	return out
}
func setTestAssembly(t *testing.T, m *LiveManager, a otlpAssembly) {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	a.DueAt = time.Now().Add(-time.Second)
	if err := m.appendLocked(liveJournal{Type: "otlp.updated", PlanID: a.PlanID, Assembly: &a}); err != nil {
		t.Fatal(err)
	}
}
func liveReport(t *testing.T, m *LiveManager, ctx context.Context, id string, from, to time.Time) (LiveReport, error) {
	t.Helper()
	report, err := m.Report(ctx, id, from, to)
	if err != nil {
		return report, err
	}
	for i := range report.Windows {
		r := &report.Windows[i]
		full, e := m.Engine.Store.Get(r.ID)
		if e != nil {
			t.Fatal(e)
		}
		r.Scope = full.Scope
		r.Cases = []CaseResult{}
		if e = full.ForEachCase(func(c CaseResult) error { r.Cases = append(r.Cases, c); return nil }); e != nil {
			t.Fatal(e)
		}
	}
	if len(report.Windows) > 0 {
		current := report.Windows[len(report.Windows)-1]
		report.Current = &current
	}
	return report, nil
}

func liveWindow(t *testing.T, m *LiveManager, ctx context.Context, id string) (Run, error) {
	t.Helper()
	r, err := m.Window(ctx, id)
	if err != nil {
		return r, err
	}
	r.Cases = []CaseResult{}
	err = r.ForEachCase(func(c CaseResult) error { r.Cases = append(r.Cases, c); return nil })
	return r, err
}

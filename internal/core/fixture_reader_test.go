package core

import (
	"context"
	"testing"
)

// Small semantic fixtures materialize explicitly; production queries never do.
func hydrateFixture(t *testing.T, r Run) Run {
	t.Helper()
	if r.caseSource == nil {
		return r
	}
	cases := []CaseResult{}
	if err := r.ForEachCase(func(c CaseResult) error { cases = append(cases, c); return nil }); err != nil {
		t.Fatal(err)
	}
	r.Cases = cases
	r.caseSource = nil
	return r
}
func executeFixture(t *testing.T, e *Engine, ctx context.Context, r Run) (Run, error) {
	got, err := e.Execute(ctx, r)
	if err == nil {
		got = hydrateFixture(t, got)
	}
	return got, err
}
func getFixture(t *testing.T, s *Store, id string) (Run, error) {
	got, err := s.Get(id)
	if err == nil {
		got = hydrateFixture(t, got)
	}
	return got, err
}
func importFixture(t *testing.T, path string) (Run, error) {
	got, err := ImportFile(path)
	if err == nil {
		got = hydrateFixture(t, got)
	}
	return got, err
}

package core

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestV2ConcurrentImportOnlyCommitsOnce(t *testing.T) {
	e, _, _ := fixture(t)
	r := runPlan(t, e, "smoke")
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.ImportRun(r)
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	succeeded := 0
	for err := range results {
		if err == nil {
			succeeded++
		}
	}
	if succeeded != 1 {
		t.Fatalf("imported %d times", succeeded)
	}
	page, err := s.Cases(r.ID, 0, 100, CaseFilter{})
	if err != nil || page.Total != r.Completed {
		t.Fatalf("duplicate cases: %+v %v", page, err)
	}
}

func TestV2PortableBeyond64MiBAndBoundedPages(t *testing.T) {
	e, _, _ := fixture(t)
	seed := runPlan(t, e, "smoke")
	sample := seed.Cases[0]
	r := runMetadata(seed)
	r.Total = 34
	r.Completed = 34
	r.Outputs = []ProcessorOutput{}
	r.Conclusions = []Conclusion{}
	payload := strings.Repeat("x", 1<<20)
	r.caseSource = func(visit func(CaseResult) error) error {
		for i := 0; i < r.Total; i++ {
			c := sample
			c.Case.ID = fmt.Sprintf("large-%d", i)
			c.Case.Response = payload
			c.Result.Response = payload
			c.Evidence = []Evidence{}
			c.Metrics = []Metric{}
			if err := visit(c); err != nil {
				return err
			}
		}
		return nil
	}
	dest := filepath.Join(t.TempDir(), "large.seven.eval")
	if err := Export(r, dest); err != nil {
		t.Fatal(err)
	}
	z, err := zip.OpenReader(dest)
	if err != nil {
		t.Fatal(err)
	}
	var expanded uint64
	results := 0
	for _, f := range z.File {
		expanded += f.UncompressedSize64
		if strings.HasPrefix(f.Name, "results/") {
			results++
			if f.UncompressedSize64 > MaxPartBytes {
				t.Fatal("unbounded part")
			}
		}
	}
	z.Close()
	if expanded <= 64<<20 || results < 2 {
		t.Fatal("fixture did not exercise partitioning", expanded, results)
	}
	imported, err := ImportFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if len(imported.Cases) != 0 {
		t.Fatal("eager hydration")
	}
	count := 0
	if err = imported.ForEachCase(func(c CaseResult) error {
		count++
		if len(c.Result.Response.(string)) != len(payload) {
			t.Fatal("truncated evidence")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if count != r.Total {
		t.Fatal(count)
	}
}

func TestV2PartialImportPreservesUnexecutedInputs(t *testing.T) {
	e, s, root := fixture(t)
	put(t, root, "datasets/sample/cases.jsonl", "{\"id\":\"a\",\"inputs\":{},\"response\":\"one\"}\n{\"id\":\"b\",\"inputs\":{},\"response\":\"two\"}\n")
	r, err := e.Prepare(context.Background(), "smoke")
	if err != nil {
		t.Fatal(err)
	}
	r.Status = "cancelled"
	if err = s.Commit(&r, "run.cancelled", nil); err != nil {
		t.Fatal(err)
	}
	s.attachSources(&r)
	if r.Status != "cancelled" || r.Completed != 0 {
		t.Fatal("unexpected cancellation")
	}
	bundle := filepath.Join(t.TempDir(), "partial.seven.eval")
	if err = Export(r, bundle); err != nil {
		t.Fatal(err)
	}
	imported, err := ImportFile(bundle)
	if err != nil {
		t.Fatal(err)
	}
	target, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	copied, err := target.ImportRun(imported)
	if err != nil {
		t.Fatal(err)
	}
	inputs := 0
	if err = copied.ForEachInput(func(c Case) error { inputs++; return nil }); err != nil {
		t.Fatal(err)
	}
	if inputs != 2 || copied.Completed != 0 {
		t.Fatal("pending inputs lost")
	}
	second := filepath.Join(t.TempDir(), "again.seven.eval")
	if err = Export(copied, second); err != nil {
		t.Fatal(err)
	}
	if _, err = ImportFile(second); err != nil {
		t.Fatal(err)
	}
	original, err := s.Get(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(original)
	if strings.Contains(string(body), "\"cases\":") {
		t.Fatal("run metadata embeds evidence")
	}
}

func TestV2ImportSourceReplacementRejected(t *testing.T) {
	e, _, _ := fixture(t)
	r := runPlan(t, e, "smoke")
	dir := t.TempDir()
	path := filepath.Join(dir, "run.seven.eval")
	if err := Export(r, path); err != nil {
		t.Fatal(err)
	}
	imported, err := ImportFile(path)
	if err != nil {
		t.Fatal(err)
	}
	r.Cases[0].Result.Response = "replacement"
	r.Cases[0].Metrics = []Metric{}
	other := filepath.Join(dir, "other.seven.eval")
	if err = Export(r, other); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(other, path); err != nil {
		t.Fatal(err)
	}
	if err = imported.ForEachCase(func(CaseResult) error { return nil }); err == nil {
		t.Fatal("changed archive accepted")
	}
}

func TestV2CasePagingAndDetail(t *testing.T) {
	e, s, _ := fixture(t)
	run := runPlan(t, e, "smoke")
	meta, err := s.Get(run.ID)
	if err != nil || len(meta.Cases) != 0 {
		t.Fatal("metadata hydrated", err)
	}
	page, err := s.Cases(run.ID, 0, 1, CaseFilter{})
	if err != nil || page.Total != run.Completed || len(page.Items) != 1 {
		t.Fatal("page", page, err)
	}
	detail, err := s.Case(run.ID, page.Items[0].ID)
	if err != nil || Digest(detail) != Digest(run.Cases[0]) {
		t.Fatal("detail changed", err)
	}
	if _, err = s.Cases(run.ID, 0, 101, CaseFilter{}); err == nil {
		t.Fatal("unbounded page accepted")
	}
}

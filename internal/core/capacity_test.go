package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Run with -benchtime=1x: measures persistence only, not evaluator throughput.
func BenchmarkCapacityPersistence(b *testing.B) {
	for _, count := range []int{100, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				s, err := NewStore(b.TempDir())
				if err != nil {
					b.Fatal(err)
				}
				r := Run{ID: "capacity", Status: "running"}
				if err = s.Commit(&r, "run.started", nil); err != nil {
					b.Fatal(err)
				}
				for i := 0; i < count; i++ {
					c := CaseResult{Case: Case{ID: fmt.Sprint(i)}, Status: "completed", Result: SubjectResult{Response: strings.Repeat("x", 1024)}}
					for j := 0; j < 10; j++ {
						c.Metrics = append(c.Metrics, Metric{ID: fmt.Sprintf("%d-%d", i, j), CaseID: c.Case.ID, MetricID: fmt.Sprint(j), Status: "observed", Value: 1, Evaluator: "capacity@1"})
					}
					r.Cases = append(r.Cases, c)
					r.Completed = len(r.Cases)
					if err = s.Commit(&r, "case.completed", c); err != nil {
						b.Fatal(err)
					}
				}
				r.Status = "completed"
				if err = s.Commit(&r, "run.finished", nil); err != nil {
					b.Fatal(err)
				}
				for _, name := range []string{"events.jsonl", "result.json"} {
					st, err := os.Stat(filepath.Join(s.Root, "runs", r.ID, name))
					if err != nil {
						b.Fatal(err)
					}
					b.ReportMetric(float64(st.Size()), name+"-bytes")
				}
				if err = s.Close(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

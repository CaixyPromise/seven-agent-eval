package core

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// reduce invokes one version-2 reducer through initialize/update/finalize.
// State is JSON, persisted between operations; no worker memory is assumed.
func (e *Engine) reduce(ctx context.Context, r *Run, ref, digest string) (ProcessorOutput, error) {
	var output ProcessorOutput
	dir, err := os.MkdirTemp(e.Store.Root, ".processor-")
	if err != nil {
		return output, err
	}
	defer os.RemoveAll(dir)
	state := any(nil)
	invoke := func(phase string, batch []CaseResult, out any) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		input := map[string]any{"phase": phase, "state": state, "cases": batch, "workspace": dir, "total": r.Total, "completed": r.Completed, "input_digest": digest, "episodes": r.Episodes, "window": r.Scope, "comparison": r.Comparison, "candidates": r.Effective.Plan.Candidates}
		return e.extension(ctx, r, ref, "processor", "processor/reduce", input, out)
	}
	if err := invoke("initialize", nil, &state); err != nil {
		return output, err
	}
	checkpoint := func() error {
		b, err := json.Marshal(state)
		if err != nil {
			return err
		}
		if len(b) > MaxMetadataBytes {
			return errors.New("processor state exceeds limit")
		}
		return atomicJSON(filepath.Join(dir, "state.json"), state)
	}
	if err := checkpoint(); err != nil {
		return output, err
	}
	batch := []CaseResult{}
	bytes := 0
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := invoke("update", batch, &state); err != nil {
			return err
		}
		batch = nil
		bytes = 0
		return checkpoint()
	}
	err = r.ForEachCase(func(c CaseResult) error {
		b, err := json.Marshal(c)
		if err != nil {
			return err
		}
		if len(batch) >= CaseBatchSize || bytes+len(b) > MaxMetadataBytes {
			if err := flush(); err != nil {
				return err
			}
		}
		batch = append(batch, c)
		bytes += len(b)
		return nil
	})
	if err != nil {
		return output, err
	}
	if err := flush(); err != nil {
		return output, err
	}
	if err := invoke("finalize", nil, &output); err != nil {
		return output, err
	}
	if err := validateProcessorOutput(output); err != nil {
		return output, err
	}
	output.InputDigest = digest
	return output, nil
}

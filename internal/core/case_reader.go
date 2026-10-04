package core

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// ForEachCase streams evidence from its owner. It never hydrates a Run.
func (r Run) ForEachCase(visit func(CaseResult) error) error {
	if r.caseSource != nil {
		return r.caseSource(visit)
	}
	for _, c := range r.Cases {
		if err := visit(c); err != nil {
			return err
		}
	}
	if len(r.Cases) != r.Completed {
		return errors.New("case source unavailable")
	}
	return nil
}

func (r Run) CaseDigest() (string, error) {
	h := sha256.New()
	err := r.ForEachCase(func(c CaseResult) error {
		b, err := json.Marshal(c)
		if err != nil {
			return err
		}
		h.Write(b)
		h.Write([]byte{'\n'})
		return nil
	})
	return hex.EncodeToString(h.Sum(nil)), err
}

func runMetadata(r Run) Run { r.Cases = nil; r.caseSource = nil; r.inputSource = nil; return r }

func (r Run) ForEachInput(visit func(Case) error) error {
	if r.inputSource != nil {
		return r.inputSource(visit)
	}
	return r.ForEachCase(func(c CaseResult) error { return visit(c.Case) })
}

func (s *Store) attachSources(r *Run) {
	id := r.ID
	r.caseSource = func(visit func(CaseResult) error) error { return s.WalkCases(id, visit) }
	path := filepath.Join(s.Root, "runs", id, "inputs", "cases.jsonl")
	if _, err := os.Stat(path); err == nil {
		r.inputSource = func(visit func(Case) error) error {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()
			scanner := bufio.NewScanner(f)
			scanner.Buffer(make([]byte, 4096), inputRecordBytes)
			for scanner.Scan() {
				var c Case
				if err := decodeJSON(scanner.Bytes(), &c); err != nil {
					return err
				}
				if err := visit(c); err != nil {
					return err
				}
			}
			return scanner.Err()
		}
	}
}

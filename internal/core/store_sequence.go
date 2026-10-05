package core

import (
	"encoding/json"
	"errors"
	"io"
	"os"
)

const MaxActiveRunSequences = 128

// The cache is bounded even when live window revisions create many runs.
// On eviction, read only the last bounded journal record, not the whole run.
func (s *Store) rememberSequence(id string, sequence int) {
	if _, ok := s.sequence[id]; !ok && len(s.sequence) >= MaxActiveRunSequences {
		for key := range s.sequence {
			delete(s.sequence, key)
			break
		}
	}
	s.sequence[id] = sequence
}

func lastJournalSequence(path string) (int, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return 0, err
	}
	end := stat.Size()
	if end == 0 {
		return 0, nil
	}
	for size := int64(1024); ; size *= 2 {
		size = min(size, end, int64(MaxMetadataBytes))
		b := make([]byte, size)
		if _, err = f.ReadAt(b, end-size); err != nil && err != io.EOF {
			return 0, err
		}
		if b[len(b)-1] != '\n' {
			return 0, errors.New("incomplete run journal; reopen to recover")
		}
		start := len(b) - 2
		for start >= 0 && b[start] != '\n' {
			start--
		}
		if start >= 0 || size == end {
			var receipt struct {
				Sequence int `json:"sequence"`
			}
			if err = json.Unmarshal(b[start+1:len(b)-1], &receipt); err != nil {
				return 0, err
			}
			if receipt.Sequence <= 0 {
				return 0, errors.New("invalid journal sequence")
			}
			return receipt.Sequence, nil
		}
		if size >= int64(MaxMetadataBytes) {
			return 0, errors.New("run journal record exceeds limit")
		}
	}
}

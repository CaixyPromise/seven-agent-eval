package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBoundedRunSequenceCacheAndTail(t *testing.T) {
	s := &Store{sequence: map[string]int{}}
	for i := 0; i < MaxActiveRunSequences*3; i++ {
		s.rememberSequence(strings.Repeat("x", i+1), i+1)
	}
	if len(s.sequence) != MaxActiveRunSequences {
		t.Fatal(len(s.sequence))
	}
	p := filepath.Join(t.TempDir(), "journal.jsonl")
	if err := os.WriteFile(p, []byte("{\"sequence\":1}\n{\"sequence\":2,\"body\":\""+strings.Repeat("a", 5000)+"\"}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	n, err := lastJournalSequence(p)
	if err != nil || n != 2 {
		t.Fatal(n, err)
	}
	if err = os.WriteFile(p, []byte("{\"sequence\":3}"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = lastJournalSequence(p); err == nil {
		t.Fatal("accepted unsynced tail")
	}
}

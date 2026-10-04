package core

import (
	"bufio"
	"errors"
	"io"
	"os"
	"path/filepath"
)

// readJournalLine bounds allocation even for an unterminated corrupt record.
// An ordinary partial tail is returned with EOF for recovery to discard.
func readJournalLine(r *bufio.Reader, limit int) ([]byte, error) {
	var line []byte
	for {
		fragment, err := r.ReadSlice('\n')
		if len(fragment) > limit-len(line) {
			return nil, errors.New("journal record too large")
		}
		line = append(line, fragment...)
		if err != bufio.ErrBufferFull {
			return line, err
		}
	}
}

func syncDirectory(path string) error {
	d, err := os.Open(path)
	if err != nil {
		return err
	}
	err = d.Sync()
	ce := d.Close()
	if err != nil {
		return err
	}
	return ce
}

// No caller may acknowledge an uncertain append or append again before recovery.
// Kept as a function dependency on each store for deterministic ENOSPC tests.
func appendJournal(path string, line []byte) error {
	_, statErr := os.Stat(path)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	n, err := f.Write(line)
	if err == nil && n != len(line) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = f.Sync()
	}
	ce := f.Close()
	if err == nil {
		err = ce
	}
	if err == nil && os.IsNotExist(statErr) {
		err = syncDirectory(filepath.Dir(path))
	}
	return err
}

func truncateJournal(path string, offset int64) error {
	f, err := os.OpenFile(path, os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	err = f.Truncate(offset)
	if err == nil {
		err = f.Sync()
	}
	ce := f.Close()
	if err != nil {
		return err
	}
	return ce
}

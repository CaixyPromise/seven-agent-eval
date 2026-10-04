package server

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"seven-agent-eval/internal/core"
	"time"
)

type download struct {
	dir, path, name string
	timer           *time.Timer
}

// Prepare on disk, then let the browser download natively. No multi-GB Blob.
// A bounded, single-use ticket also lets preparation errors remain in the UI.
func (s *Server) prepareDownload(w http.ResponseWriter, r *http.Request, run core.Run) {
	s.downloadMu.Lock()
	if s.downloadsClosed {
		s.downloadMu.Unlock()
		failureCode(w, "EXPORT_UNAVAILABLE", 503)
		return
	}
	if s.downloads == nil {
		s.downloads = map[string]*download{}
	}
	if len(s.downloads) >= 4 {
		s.downloadMu.Unlock()
		failureCode(w, "EXPORT_UNAVAILABLE", 429)
		return
	}
	token := rand.Text()
	entry := &download{name: run.ID + ".seven.eval"}
	s.downloads[token] = entry
	s.downloadWG.Add(1)
	s.downloadMu.Unlock()
	defer s.downloadWG.Done()
	success := false
	defer func() {
		if !success {
			s.downloadMu.Lock()
			delete(s.downloads, token)
			s.downloadMu.Unlock()
			if entry.dir != "" {
				os.RemoveAll(entry.dir)
			}
		}
	}()
	dir, err := os.MkdirTemp("", "seven-download-")
	if err != nil {
		failureCode(w, "EXPORT_UNAVAILABLE", 500)
		return
	}
	entry.dir = dir
	entry.path = filepath.Join(dir, entry.name)
	if err = core.Export(run, entry.path); err != nil {
		failureCode(w, "EXPORT_UNAVAILABLE", 409)
		return
	}
	if r.Context().Err() != nil {
		return
	}
	s.downloadMu.Lock()
	if s.downloadsClosed {
		s.downloadMu.Unlock()
		return
	}
	entry.timer = time.AfterFunc(10*time.Minute, func() {
		s.downloadMu.Lock()
		pending := s.downloads[token]
		delete(s.downloads, token)
		s.downloadMu.Unlock()
		if pending != nil {
			os.RemoveAll(pending.dir)
		}
	})
	s.downloadMu.Unlock()
	success = true
	jsonOut(w, map[string]string{"url": "/api/downloads/" + token})
}
func (s *Server) download(w http.ResponseWriter, r *http.Request) {
	s.downloadMu.Lock()
	entry := s.downloads[r.PathValue("token")]
	if entry != nil && entry.timer != nil {
		delete(s.downloads, r.PathValue("token"))
		entry.timer.Stop()
	} else {
		entry = nil
	}
	s.downloadMu.Unlock()
	if entry == nil {
		failureCode(w, "EXPORT_UNAVAILABLE", 404)
		return
	}
	defer os.RemoveAll(entry.dir)
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", entry.name))
	http.ServeFile(w, r, entry.path)
}
func (s *Server) closeDownloads() {
	s.downloadMu.Lock()
	s.downloadsClosed = true
	s.downloadMu.Unlock()
	s.downloadWG.Wait()
	s.downloadMu.Lock()
	defer s.downloadMu.Unlock()
	for token, entry := range s.downloads {
		if entry.timer != nil {
			entry.timer.Stop()
			os.RemoveAll(entry.dir)
			delete(s.downloads, token)
		}
	}
}

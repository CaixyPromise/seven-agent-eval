package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"seven-agent-eval/internal/core"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Server struct {
	Engine  *core.Engine
	Live    *core.LiveManager
	liveErr error
	WebDir  string
	mu      sync.Mutex
	active  map[string]context.CancelFunc
	wg      sync.WaitGroup
	closing bool
}

func New(e *core.Engine, web string) *Server {
	live, err := core.NewLiveManager(e)
	if err == nil {
		live.Start()
	}
	return &Server{Engine: e, Live: live, liveErr: err, WebDir: web, active: map[string]context.CancelFunc{}}
}
func (s *Server) Shutdown() {
	s.mu.Lock()
	s.closing = true
	for _, cancel := range s.active {
		cancel()
	}
	s.mu.Unlock()
	s.wg.Wait()
	if s.Live != nil {
		_ = s.Live.Close()
	}
}
func jsonOut(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func failure(w http.ResponseWriter, err error, code int) {
	productCode := "UNEXPECTED"
	switch err.Error() {
	case "run not found":
		productCode = "RUN_NOT_FOUND"
	case "run not active":
		productCode = "RUN_NOT_ACTIVE"
	case "one run at a time in phase 1":
		productCode = "RUN_BUSY"
	case "run already imported":
		productCode = "BUNDLE_EXISTS"
	case "import exceeds size limit":
		productCode = "BUNDLE_TOO_LARGE"
	case "loopback host required", "cross-origin request rejected", "cross-site request rejected":
		productCode = "ACCESS_DENIED"
	}
	failureCode(w, productCode, code)
}
func failureCode(w http.ResponseWriter, productCode string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": productCode})
}
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/assets", func(w http.ResponseWriter, r *http.Request) {
		if _, err := os.Stat(filepath.Join(s.Engine.Root, "evaluation.yaml")); os.IsNotExist(err) {
			jsonOut(w, core.Discovery{Package: core.Package{ID: "viewer", Name: "结果查看器", Version: "1"}, Assets: []core.Asset{}})
			return
		}
		d, e := core.Discover(r.Context(), s.Engine.Root, s.Engine.Worker)
		if e != nil {
			failureCode(w, "PACKAGE_INVALID", 400)
			return
		}
		// Discovery's CLI diagnostics are richer than the browser's safe product contract.
		assets := []map[string]any{}
		for _, asset := range d.Assets {
			diagnostic := ""
			if asset.Status == "invalid" {
				diagnostic = "ASSET_INVALID"
				switch {
				case asset.Message == "duplicate asset id":
					diagnostic = "ASSET_DUPLICATE"
				case strings.HasPrefix(asset.Message, "unresolved "), strings.HasPrefix(asset.Message, "invalid referenced "):
					diagnostic = "ASSET_REFERENCE"
				case strings.Contains(asset.Message, "TypeScript runtime"), strings.Contains(asset.Message, "require a Python descriptor"):
					diagnostic = "ASSET_RUNTIME"
				}
			}
			descriptor := map[string]any{}
			for _, key := range []string{"id", "kind", "mode", "dataset", "source", "mapping", "format", "preset", "evaluators", "processors", "conclusions"} {
				if value, ok := asset.Descriptor[key]; ok {
					descriptor[key] = value
				}
			}
			if asset.Kind == "mapping" {
				if fields, ok := asset.Descriptor["fields"].(map[string]any); ok {
					descriptor["field_count"] = len(fields)
				}
			}
			assets = append(assets, map[string]any{"kind": asset.Kind, "id": asset.ID, "version": asset.Version, "source": asset.Source, "path": asset.Path, "status": asset.Status, "diagnostic_code": diagnostic, "descriptor": descriptor})
		}
		jsonOut(w, map[string]any{"package": d.Package, "assets": assets})
	})
	mux.HandleFunc("GET /api/runs", func(w http.ResponseWriter, r *http.Request) {
		runs, e := s.Engine.Store.List()
		if e != nil {
			failure(w, e, 500)
			return
		}
		items := []map[string]any{}
		for _, v := range runs {
			items = append(items, map[string]any{"id": v.ID, "package": v.Package, "created_at": v.CreatedAt, "status": v.Status, "total": v.Total, "completed": v.Completed, "message": v.Message})
		}
		jsonOut(w, items)
	})
	mux.HandleFunc("POST /api/runs", s.start)
	mux.HandleFunc("GET /api/runs/{id}", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Engine.Store.Get(r.PathValue("id"))
		if e != nil {
			failure(w, errors.New("run not found"), 404)
			return
		}
		jsonOut(w, v)
	})
	mux.HandleFunc("POST /api/runs/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		cancel, ok := s.active[r.PathValue("id")]
		if ok {
			cancel()
		}
		s.mu.Unlock()
		if !ok {
			failure(w, errors.New("run not active"), 409)
			return
		}
		jsonOut(w, map[string]bool{"cancelled": true})
	})
	mux.HandleFunc("GET /api/runs/{id}/events", s.events)
	mux.HandleFunc("GET /api/runs/{id}/report", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Engine.Store.Get(r.PathValue("id"))
		if e != nil {
			failure(w, errors.New("run not found"), 404)
			return
		}
		b, e := core.Report(v)
		if e != nil {
			failure(w, e, 500)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
		w.Write(b)
	})
	mux.HandleFunc("GET /api/runs/{id}/export", s.export)
	mux.HandleFunc("POST /api/import", s.importRun)
	mux.HandleFunc("GET /api/live/plans", s.livePlans)
	mux.HandleFunc("GET /api/live/plans/{id}/report", s.liveReport)
	mux.HandleFunc("GET /api/live/plans/{id}/events", s.liveEvents)
	mux.HandleFunc("POST /api/live/plans/{id}/pause", s.livePause)
	mux.HandleFunc("POST /api/live/plans/{id}/resume", s.liveResume)
	mux.HandleFunc("GET /api/live/windows/{id}", s.liveWindow)
	mux.HandleFunc("GET /api/live/windows/{id}/export", s.liveExport)
	mux.HandleFunc("POST /api/ingest/events", s.ingest)
	mux.HandleFunc("POST /v1/traces", s.ingestOTLP)
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		jsonOut(w, map[string]string{"status": "ok", "version": core.Version})
	})
	mux.Handle("/", http.FileServer(http.Dir(s.WebDir)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, e := net.SplitHostPort(host); e == nil {
			host = h
		}
		ip := net.ParseIP(host)
		if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			failure(w, errors.New("loopback host required"), 403)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, e := url.Parse(origin)
			if e != nil || u.Host != r.Host || u.Scheme != "http" {
				failure(w, errors.New("cross-origin request rejected"), 403)
				return
			}
		}
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			failure(w, errors.New("cross-site request rejected"), 403)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			w.Header().Set("Cache-Control", "no-cache")
		}
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
		mux.ServeHTTP(w, r)
	})
}
func (s *Server) liveUnavailable(w http.ResponseWriter) bool {
	if s.Live != nil {
		return false
	}
	failureCode(w, "LIVE_UNAVAILABLE", http.StatusServiceUnavailable)
	return true
}
func liveCode(err error) (string, int) {
	var planErr *core.PlanError
	if errors.As(err, &planErr) {
		return "LIVE_PLAN_INVALID", http.StatusBadRequest
	}
	switch err.Error() {
	case "plan is not a stream":
		return "LIVE_PLAN_INVALID", http.StatusBadRequest
	case "ingest batch too large":
		return "INGEST_BATCH_TOO_LARGE", http.StatusRequestEntityTooLarge
	case "ingest queue full":
		return "QUEUE_FULL", http.StatusTooManyRequests
	case "window not found":
		return "WINDOW_NOT_FOUND", http.StatusNotFound
	case "OTLP plan mapping missing", "OTLP turn mapping missing", "OTLP root mapping missing", "OTLP contains no mappable trace", "event does not contain the plan evaluation unit":
		return "OTLP_MAPPING_REQUIRED", http.StatusBadRequest
	case "OTLP batch contains multiple plans":
		return "OTLP_PLAN_CONFLICT", http.StatusBadRequest
	case "invalid report range":
		return "WINDOW_RANGE_INVALID", http.StatusBadRequest
	case "ingest request requires plan and events":
		return "INGEST_INVALID", http.StatusBadRequest
	default:
		return "LIVE_UNAVAILABLE", http.StatusServiceUnavailable
	}
}
func liveFailure(w http.ResponseWriter, err error) {
	code, status := liveCode(err)
	failureCode(w, code, status)
}
func (s *Server) livePlans(w http.ResponseWriter, r *http.Request) {
	if s.liveUnavailable(w) {
		return
	}
	plans, err := s.Live.Plans(r.Context())
	if err != nil {
		liveFailure(w, err)
		return
	}
	jsonOut(w, plans)
}
func liveRange(value string) (time.Duration, bool) {
	switch value {
	case "15m":
		return 15 * time.Minute, true
	case "1h", "":
		return time.Hour, true
	case "6h":
		return 6 * time.Hour, true
	case "24h":
		return 24 * time.Hour, true
	}
	return 0, false
}
func (s *Server) liveReport(w http.ResponseWriter, r *http.Request) {
	if s.liveUnavailable(w) {
		return
	}
	d, ok := liveRange(r.URL.Query().Get("range"))
	if !ok {
		failureCode(w, "WINDOW_RANGE_INVALID", 400)
		return
	}
	to := time.Now().UTC()
	report, err := s.Live.Report(r.Context(), r.PathValue("id"), to.Add(-d), to)
	if err != nil {
		liveFailure(w, err)
		return
	}
	jsonOut(w, report)
}
func (s *Server) livePause(w http.ResponseWriter, r *http.Request) {
	if s.liveUnavailable(w) {
		return
	}
	v, err := s.Live.SetPaused(r.Context(), r.PathValue("id"), true)
	if err != nil {
		liveFailure(w, err)
		return
	}
	jsonOut(w, v)
}
func (s *Server) liveResume(w http.ResponseWriter, r *http.Request) {
	if s.liveUnavailable(w) {
		return
	}
	v, err := s.Live.SetPaused(r.Context(), r.PathValue("id"), false)
	if err != nil {
		liveFailure(w, err)
		return
	}
	jsonOut(w, v)
}
func (s *Server) liveWindow(w http.ResponseWriter, r *http.Request) {
	if s.liveUnavailable(w) {
		return
	}
	v, err := s.Live.Window(r.Context(), r.PathValue("id"))
	if err != nil {
		liveFailure(w, err)
		return
	}
	jsonOut(w, v)
}
func (s *Server) liveExport(w http.ResponseWriter, r *http.Request) {
	if s.liveUnavailable(w) {
		return
	}
	run, err := s.Live.Window(r.Context(), r.PathValue("id"))
	if err != nil {
		liveFailure(w, err)
		return
	}
	s.exportRun(w, r, run)
}
func (s *Server) liveEvents(w http.ResponseWriter, r *http.Request) {
	if s.liveUnavailable(w) {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		failureCode(w, "LIVE_UNAVAILABLE", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	previous := ""
	for {
		d, ok := liveRange(r.URL.Query().Get("range"))
		if !ok {
			d = time.Hour
		}
		to := time.Now().UTC()
		report, err := s.Live.Report(r.Context(), r.PathValue("id"), to.Add(-d), to)
		if err != nil {
			return
		}
		body, _ := json.Marshal(report)
		if string(body) != previous {
			fmt.Fprintf(w, "event: snapshot\ndata: %s\n\n", body)
			flusher.Flush()
			previous = string(body)
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}
func (s *Server) ingest(w http.ResponseWriter, r *http.Request) {
	if s.liveUnavailable(w) {
		return
	}
	var batch core.IngestBatch
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, int64(core.MaxLiveBatch*core.MaxLiveEventBytes)+4096))
	decoder.UseNumber()
	if err := decoder.Decode(&batch); err != nil {
		failureCode(w, "INGEST_INVALID", 400)
		return
	}
	result, err := s.Live.Ingest(r.Context(), batch)
	if err != nil {
		liveFailure(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(result)
}
func (s *Server) ingestOTLP(w http.ResponseWriter, r *http.Request) {
	if s.liveUnavailable(w) {
		return
	}
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		failureCode(w, "OTLP_ENCODING_UNSUPPORTED", http.StatusUnsupportedMediaType)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 32<<20))
	if err != nil {
		failureCode(w, "INGEST_BATCH_TOO_LARGE", 413)
		return
	}
	result, err := s.Live.IngestOTLP(r.Context(), body)
	if err != nil {
		liveFailure(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(result)
}
func (s *Server) start(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Plan string `json:"plan"`
	}
	if e := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); e != nil {
		failure(w, errors.New("invalid request"), 400)
		return
	}
	s.mu.Lock()
	if s.closing || len(s.active) > 0 {
		s.mu.Unlock()
		failure(w, errors.New("one run at a time in phase 1"), 409)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.active["preparing"] = cancel
	s.wg.Add(1) // Preparation is part of the lifecycle drained before closing storage.
	s.mu.Unlock()
	run, e := s.Engine.Prepare(ctx, in.Plan)
	s.mu.Lock()
	delete(s.active, "preparing")
	if e != nil {
		s.mu.Unlock()
		cancel()
		s.wg.Done()
		var validation *core.PlanError
		if errors.As(e, &validation) {
			failureCode(w, "PLAN_INVALID", 400)
		} else if run.ID != "" {
			failureCode(w, "START_UNKNOWN", 500)
		} else {
			failureCode(w, "START_UNAVAILABLE", 503)
		}
		return
	}
	s.active[run.ID] = cancel
	s.mu.Unlock()
	go func() {
		defer s.wg.Done()
		defer cancel()
		_, err := s.Engine.Execute(ctx, run)
		var projection *core.ProjectionError
		if err != nil && !errors.As(err, &projection) {
			latest, er := s.Engine.Store.Get(run.ID)
			if er == nil {
				latest.Status = "failed"
				latest.Message = "Execution persistence or protocol failure"
				_ = s.Engine.Store.Commit(&latest, "run.failed", nil)
			}
		}
		s.mu.Lock()
		delete(s.active, run.ID)
		s.mu.Unlock()
	}()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	jsonOut(w, run)
}
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		failure(w, errors.New("stream unavailable"), 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	cursor, _ := strconv.Atoi(r.Header.Get("Last-Event-ID"))
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	previous := ""
	for {
		v, e := s.Engine.Store.Get(r.PathValue("id"))
		if e != nil {
			return
		}
		data, _ := json.Marshal(map[string]any{"id": v.ID, "status": v.Status, "total": v.Total, "completed": v.Completed})
		if string(data) != previous {
			cursor++
			fmt.Fprintf(w, "id: %d\nevent: snapshot\ndata: %s\n\n", cursor, data)
			flusher.Flush()
			previous = string(data)
		}
		if v.Status != "running" && v.Status != "processing" {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}
func (s *Server) export(w http.ResponseWriter, r *http.Request) {
	run, e := s.Engine.Store.Get(r.PathValue("id"))
	if e != nil {
		failure(w, errors.New("run not found"), 404)
		return
	}
	s.exportRun(w, r, run)
}
func (s *Server) exportRun(w http.ResponseWriter, r *http.Request, run core.Run) {
	dir, e := os.MkdirTemp("", "seven-export-")
	if e != nil {
		failure(w, e, 500)
		return
	}
	defer os.RemoveAll(dir)
	p := filepath.Join(dir, run.ID+".seven.eval")
	if e = core.Export(run, p); e != nil {
		failureCode(w, "EXPORT_UNAVAILABLE", 409)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", run.ID+".seven.eval"))
	http.ServeFile(w, r, p)
}
func (s *Server) importRun(w http.ResponseWriter, r *http.Request) {
	b, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<20))
	if e != nil {
		failure(w, errors.New("import exceeds size limit"), 400)
		return
	}
	v, e := core.Import(bytes.NewReader(b), int64(len(b)))
	if e != nil {
		failureCode(w, "BUNDLE_INVALID", 400)
		return
	}
	if _, e = s.Engine.Store.Get(v.ID); e == nil {
		failure(w, errors.New("run already imported"), 409)
		return
	}
	if e = s.Engine.Store.Commit(&v, "run.imported", nil); e != nil {
		failure(w, e, 500)
		return
	}
	jsonOut(w, v)
}
func ListenAddress(addr string) error {
	host, _, e := net.SplitHostPort(addr)
	if e != nil {
		return e
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil || !ip.IsLoopback() {
		return errors.New("phase 1 server requires explicit loopback address")
	}
	return nil
}

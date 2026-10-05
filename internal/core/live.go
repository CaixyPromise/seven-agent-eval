package core

// The online path is deliberately a small durable sidecar. It receives evidence
// that has already happened, journals before acknowledging it, and uses the same
// evaluator/processor/conclusion seams as replay. No live code path invokes a
// Subject.

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"seven-agent-eval/protocol"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type liveRecord struct {
	Unit               string            `json:"unit"`
	WindowStart        time.Time         `json:"window_start"`
	DecisionIdentities map[string]string `json:"decision_identities,omitempty"`
	Expired            bool              `json:"expired,omitempty"`
	ScoredSequence     int               `json:"scored_sequence,omitempty"`
	Sequence           int               `json:"sequence"`
	WindowTime         time.Time         `json:"window_time"`
	StartedAt          *time.Time        `json:"started_at,omitempty"`
	Decisions          map[string]string `json:"decisions,omitempty"`
	PlanID             string            `json:"plan_id"`
	Event              ProductionEvent   `json:"event"`
	Digest             string            `json:"digest"`
	ReceivedAt         time.Time         `json:"received_at"`
	ScoredAt           *time.Time        `json:"scored_at,omitempty"`
	Case               *CaseResult       `json:"case,omitempty"`
}
type liveJournal struct {
	ExpiredScope *ExpiredWindow  `json:"expired_scope,omitempty"`
	WindowID     string          `json:"window_id,omitempty"`
	Retention    *RetentionAudit `json:"retention,omitempty"`
	Assembly     *otlpAssembly   `json:"assembly,omitempty"`
	Sequence     int             `json:"sequence"`
	Type         string          `json:"type"`
	Time         time.Time       `json:"time"`
	PlanID       string          `json:"plan_id,omitempty"`
	Record       *liveRecord     `json:"record,omitempty"`
}
type LivePlanStatus struct {
	ProjectionDegraded bool   `json:"projection_degraded"`
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Unit               string `json:"unit"`
	Paused             bool   `json:"paused"`
	Pending            int    `json:"pending"`
	Storage            int64  `json:"storage_bytes"`
}
type LivePoint struct {
	Unscored      int       `json:"unscored"`
	Time          time.Time `json:"time"`
	Coverage      *float64  `json:"coverage,omitempty"`
	Received      int       `json:"received"`
	Scored        int       `json:"scored"`
	NotComputable int       `json:"not_computable"`
	Failed        int       `json:"failed"`
	Pending       int       `json:"pending"`
	P50LatencyMS  *float64  `json:"p50_latency_ms,omitempty"`
	P95LatencyMS  *float64  `json:"p95_latency_ms,omitempty"`
}
type LiveReport struct {
	ExpiredWindows int            `json:"expired_windows,omitempty"`
	Summary        *WindowScope   `json:"summary,omitempty"`
	BacklogSeries  []BacklogPoint `json:"backlog_series"`
	Plan           LivePlanStatus `json:"plan"`
	From           time.Time      `json:"from"`
	To             time.Time      `json:"to"`
	Current        *Run           `json:"current,omitempty"`
	Windows        []Run          `json:"windows"`
	Series         []LivePoint    `json:"series"`
}
type LiveManager struct {
	rangeCacheKey      string
	rangeCacheSummary  *WindowScope
	rangeCacheBacklog  []BacklogPoint
	compactPublish     func(string, string) error
	projectionDirty    bool
	reportMu           sync.Mutex
	projectionDegraded bool
	journalFailure     error
	appendRecord       func(string, []byte) error
	Engine             *Engine
	root               string
	db                 *sql.DB
	mu                 sync.Mutex
	seq                int
	plans              map[string]Run
	paused             map[string]bool
	wake               chan struct{}
	stop               context.CancelFunc
	wg                 sync.WaitGroup
}

func NewLiveManager(engine *Engine) (*LiveManager, error) {
	root := filepath.Join(engine.Store.Root, "live")
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(root, "projection.sqlite"))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(liveDDL); err != nil {
		db.Close()
		return nil, err
	}
	// Scratch snapshots survive an in-process projection repair, but never a
	// process restart. Active readers must retain their frozen evidence on repair.
	if _, err = db.Exec("DELETE FROM live_snapshots"); err != nil {
		db.Close()
		return nil, err
	}
	m := &LiveManager{Engine: engine, root: root, db: db, plans: map[string]Run{}, paused: map[string]bool{}, wake: make(chan struct{}, 1)}
	if err = m.rebuild(); err != nil {
		db.Close()
		return nil, err
	}
	return m, nil
}
func (m *LiveManager) Close() error {
	if m.stop != nil {
		m.stop()
		m.wg.Wait()
	}
	return m.db.Close()
}
func (m *LiveManager) Start() {
	if m.stop != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.stop = cancel
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			m.flushOTLP(ctx)
			m.scorePending(ctx)
			select {
			case <-ctx.Done():
				return
			case <-m.wake:
			case <-ticker.C:
			}
		}
	}()
}
func (m *LiveManager) signal() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}
func liveKey(planID, source, id string) string { return planID + "\x00" + source + "\x00" + id }

// cloneLiveRun gives each receiver, scorer and report construction a private
// copy of map-backed catalogs/configuration. Processor/redaction work is allowed
// to normalize its private result, but must never mutate the cached stream
// recipe observed by another goroutine.
func cloneLiveRun(in Run) Run {
	b, err := json.Marshal(in)
	if err != nil {
		return Run{}
	}
	var out Run
	if decodeJSON(b, &out) != nil {
		return Run{}
	}
	out.caseSource = in.caseSource
	out.inputSource = in.inputSource
	return out
}

func unitKey(plan Plan, e ProductionEvent) (string, error) {
	if plan.Stream == nil {
		return "", errors.New("stream configuration missing")
	}
	if plan.Stream.Unit == "trace" && e.TraceID != "" {
		return "trace:" + Digest([]string{e.Source, e.Project, e.TraceID}), nil
	}
	if plan.Stream.Unit == "turn" && e.TurnID != "" {
		return "turn:" + Digest([]string{e.Source, e.Project, e.TraceID, e.TurnID}), nil
	}
	return "", errors.New("event does not contain the plan evaluation unit")
}
func (m *LiveManager) plan(ctx context.Context, id string) (Run, error) {
	m.mu.Lock()
	cached, ok := m.plans[id]
	m.mu.Unlock()
	if ok {
		return cloneLiveRun(cached), nil
	}
	r, err := m.Engine.PrepareStream(ctx, id)
	if err != nil {
		return Run{}, err
	}
	m.mu.Lock()
	if len(m.plans) >= MaxLivePlans {
		m.mu.Unlock()
		return Run{}, errors.New("too many active stream plans")
	}
	m.plans[id] = r
	m.mu.Unlock()
	return cloneLiveRun(r), nil
}
func (m *LiveManager) appendLocked(ev liveJournal) error {
	if err := m.ensureProjectionLocked(); err != nil {
		return err
	}
	if m.journalFailure != nil {
		return errors.New("live journal requires recovery")
	}
	ev.Sequence = m.seq + 1
	ev.Time = time.Now().UTC()
	b, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	journalPath := filepath.Join(m.root, "events.jsonl")
	write := m.appendRecord
	if write == nil {
		write = appendJournal
	}
	if len(b)+1 > 64<<20 {
		return errors.New("live journal record too large")
	}
	if err = write(journalPath, append(b, '\n')); err != nil {
		m.journalFailure = err
		return err
	}
	m.seq = ev.Sequence
	tx, err := m.db.Begin()
	if err == nil {
		err = m.project(tx, ev, b)
		if err == nil {
			err = commitProjection(m.db, tx)
		} else {
			_ = tx.Rollback()
		}
	}
	if err != nil {
		log.Printf("online projection failed at durable sequence %d: %v", ev.Sequence, err)
		m.projectionDegraded = true
		m.projectionDirty = true
	}
	if ev.Type == "plan.paused" {
		m.paused[ev.PlanID] = true
	}
	if ev.Type == "plan.resumed" {
		m.paused[ev.PlanID] = false
	}
	return nil
}
func (m *LiveManager) rebuild() error {
	if _, err := m.db.Exec(liveDDL); err != nil {
		return err
	}
	tx, err := m.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, table := range []string{"live_projection", "live_records", "live_units", "live_decisions", "live_assemblies", "live_expired", "live_audits", "live_emissions", "live_controls"} {
		if _, err = tx.Exec("DELETE FROM " + table); err != nil {
			return err
		}
	}
	m.seq = 0
	m.paused = map[string]bool{}
	p := filepath.Join(m.root, "events.jsonl")
	f, err := os.Open(p)
	if os.IsNotExist(err) {
		return commitProjection(m.db, tx)
	}
	if err != nil {
		return err
	}
	defer f.Close()
	reader := bufio.NewReader(f)
	var offset int64
	for {
		line, readErr := readJournalLine(reader, 64<<20)
		if readErr == io.EOF {
			if len(line) > 0 {
				if err = truncateJournal(p, offset); err != nil {
					return err
				}
			}
			return commitProjection(m.db, tx)
		}
		if readErr != nil {
			return readErr
		}
		var ev liveJournal
		if err = decodeJSON(line, &ev); err != nil {
			return errors.New("corrupt live journal")
		}
		if ev.Sequence != m.seq+1 && !(offset == 0 && ev.Type == "journal.compacted" && ev.Sequence > 0) {
			return errors.New("live journal sequence gap")
		}
		m.seq = ev.Sequence
		if err = m.project(tx, ev, line); err != nil {
			return err
		}
		if ev.Type == "plan.paused" {
			m.paused[ev.PlanID] = true
		}
		if ev.Type == "plan.resumed" {
			m.paused[ev.PlanID] = false
		}
		offset += int64(len(line))
	}
}
func (m *LiveManager) Ingest(ctx context.Context, batch IngestBatch) (IngestResult, error) {
	if batch.PlanID == "" || len(batch.Events) == 0 {
		return IngestResult{}, errors.New("ingest request requires plan and events")
	}
	if len(batch.Events) > MaxLiveBatch {
		return IngestResult{}, errors.New("ingest batch too large")
	}
	planRun, err := m.plan(ctx, batch.PlanID)
	if err != nil {
		return IngestResult{}, err
	}
	if planRun.Effective.Plan.Mode != "stream" {
		return IngestResult{}, errors.New("plan is not a stream")
	}
	out := IngestResult{Items: make([]IngestItem, 0, len(batch.Events))}
	m.mu.Lock()
	defer m.mu.Unlock()
	pending, err := m.pendingLocked(batch.PlanID)
	if err != nil {
		return out, err
	}
	for _, event := range batch.Events {
		encoded, _ := json.Marshal(event)
		if len(encoded) > MaxLiveEventBytes {
			out.Rejected++
			out.Items = append(out.Items, IngestItem{EventID: event.EventID, Status: "rejected", Code: "EVENT_TOO_LARGE"})
			continue
		}
		if event.SchemaVersion == "" {
			event.SchemaVersion = "evaluate.event/v1"
		}
		if event.EvidenceRevision == 0 {
			event.EvidenceRevision = 1
		}
		if err := protocol.Validate("production_event", event); err != nil {
			out.Rejected++
			out.Items = append(out.Items, IngestItem{EventID: event.EventID, Status: "rejected", Code: "EVENT_INVALID"})
			continue
		}
		if _, err := unitKey(planRun.Effective.Plan, event); err != nil {
			out.Rejected++
			out.Items = append(out.Items, IngestItem{EventID: event.EventID, Status: "rejected", Code: "EVENT_UNIT_MISSING"})
			continue
		}
		key, digest := liveKey(batch.PlanID, event.Source, event.EventID), Digest(event)
		prior, lookupErr := m.recordLocked(key)
		if lookupErr != nil {
			return out, lookupErr
		}
		if prior != nil {
			if prior.Digest == digest {
				out.Duplicate++
				out.Items = append(out.Items, IngestItem{EventID: event.EventID, Status: "duplicate"})
			} else {
				out.Rejected++
				out.Items = append(out.Items, IngestItem{EventID: event.EventID, Status: "rejected", Code: "EVENT_CONFLICT"})
			}
			continue
		}
		unit, _ := unitKey(planRun.Effective.Plan, event)
		conflict, expired := false, false
		windowTime := event.EventTime
		var firstNS int64
		var unitExpired bool
		err = m.db.QueryRow("SELECT window_ns,expired FROM live_units WHERE plan=? AND unit=?", batch.PlanID, unit).Scan(&firstNS, &unitExpired)
		if err == nil {
			windowTime = time.Unix(0, firstNS).UTC()
			expired = unitExpired
		} else if !errors.Is(err, sql.ErrNoRows) {
			return out, err
		}
		var versions int
		if err = m.db.QueryRow("SELECT count(*) FROM live_records WHERE plan=? AND unit=? AND revision=?", batch.PlanID, unit, event.EvidenceRevision).Scan(&versions); err != nil {
			return out, err
		}
		conflict = versions > 0
		if conflict {
			out.Rejected++
			out.Items = append(out.Items, IngestItem{EventID: event.EventID, Status: "rejected", Code: "EVIDENCE_REVISION_CONFLICT"})
			continue
		}
		expiredRange, expiryErr := m.expiredEventLocked(batch.PlanID, windowTime)
		if expiryErr != nil {
			return out, expiryErr
		}
		if expired || expiredRange {
			out.Rejected++
			out.Items = append(out.Items, IngestItem{EventID: event.EventID, Status: "rejected", Code: "DATA_EXPIRED"})
			continue
		}
		if pending >= planRun.Effective.Plan.Stream.MaxPending {
			out.Rejected++
			out.Items = append(out.Items, IngestItem{EventID: event.EventID, Status: "rejected", Code: "QUEUE_FULL"})
			continue
		}
		record := &liveRecord{Unit: unit, WindowStart: windowTime.Truncate(time.Duration(planRun.Effective.Plan.Stream.WindowSeconds) * time.Second), WindowTime: windowTime, Sequence: m.seq + 1, PlanID: batch.PlanID, Event: event, Digest: digest, ReceivedAt: time.Now().UTC()}
		if err := m.appendLocked(liveJournal{Type: "event.accepted", PlanID: batch.PlanID, Record: record}); err != nil {
			return out, err
		}
		pending++
		out.Accepted++
		out.Items = append(out.Items, IngestItem{EventID: event.EventID, Status: "accepted"})
	}
	go m.signal()
	return out, nil
}
func (m *LiveManager) scorePending(ctx context.Context) {
	var after int
	for {
		m.mu.Lock()
		batch, err := m.recordBatchLocked("SELECT body FROM live_records r WHERE expired=0 AND scored=0 AND seq>? AND NOT EXISTS(SELECT 1 FROM live_controls c WHERE c.plan=r.plan AND c.paused=1) ORDER BY seq LIMIT ?", after, LiveReadBatch)
		m.mu.Unlock()
		if err != nil || len(batch) == 0 {
			return
		}
		for i := range batch {
			record := &batch[i]
			after = record.Sequence
			if ctx.Err() != nil {
				return
			}
			plan, err := m.plan(ctx, record.PlanID)
			if err != nil {
				continue
			}
			m.mu.Lock()
			if m.paused[record.PlanID] {
				m.mu.Unlock()
				continue
			}
			if record.StartedAt == nil {
				now := time.Now().UTC()
				record.StartedAt = &now
				record.Decisions, err = m.judgeDecisionsLocked(plan, record)
				if err == nil {
					err = m.appendLocked(liveJournal{Type: "event.started", PlanID: record.PlanID, Record: record})
				}
				if err != nil {
					m.mu.Unlock()
					return
				}
			} else {
				for ref, decision := range record.Decisions {
					if decision == "selected" {
						record.Decisions[ref] = "interrupted"
					}
				}
			}
			m.mu.Unlock()
			cctx, cancel := context.WithTimeout(ctx, time.Duration(plan.Effective.Plan.TimeoutSeconds)*time.Second)
			cctx = context.WithValue(cctx, liveDecisionKey{}, record.Decisions)
			c := record.Event.Case
			c.ID = "live-" + Digest([]string{record.PlanID, record.Unit})
			cctx = context.WithValue(cctx, liveIdentityKey{}, []any{record.PlanID, record.Unit, record.Event.EvidenceRevision})
			result, err := m.Engine.EvaluateExisting(cctx, &plan, c, record.Event.Result)
			cancel()
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				continue
			}
			now := time.Now().UTC()
			record.Case = &result
			record.ScoredAt = &now
			m.mu.Lock()
			record.ScoredSequence = m.seq + 1
			err = m.appendLocked(liveJournal{Type: "event.scored", PlanID: record.PlanID, Record: record})
			m.mu.Unlock()
			if err != nil {
				return
			}
			m.signal()
		}
	}
}
func percentile(values []float64, q float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	sort.Float64s(values)
	v := values[max(0, int(math.Ceil(q*float64(len(values))))-1)]
	return &v
}
func (m *LiveManager) plansFor(ctx context.Context) ([]LivePlanStatus, error) {
	d, err := Discover(ctx, m.Engine.Root, m.Engine.Worker)
	if err != nil {
		return nil, err
	}
	// Resolve before locking state: descriptor calls may run Python and must not
	// block a receiver that is waiting to append an acknowledged event.
	for _, a := range d.Assets {
		if a.Kind == "plan" && a.Status == "valid" {
			_, _ = m.plan(ctx, a.ID)
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	stat, _ := os.Stat(filepath.Join(m.root, "events.jsonl"))
	size := int64(0)
	if stat != nil {
		size = stat.Size()
	}
	out := []LivePlanStatus{}
	for id, r := range m.plans {
		if r.Effective.Plan.Mode == "stream" {
			out = append(out, m.statusLocked(id, r))
			out[len(out)-1].Storage = size
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
func (m *LiveManager) Plans(ctx context.Context) ([]LivePlanStatus, error) { return m.plansFor(ctx) }
func (m *LiveManager) SetPaused(ctx context.Context, id string, paused bool) (LivePlanStatus, error) {
	plan, err := m.plan(ctx, id)
	if err != nil {
		return LivePlanStatus{}, err
	}
	if plan.Effective.Plan.Mode != "stream" {
		return LivePlanStatus{}, errors.New("plan is not a stream")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.paused[id] == paused {
		return m.statusLocked(id, plan), nil
	}
	typ := "plan.resumed"
	if paused {
		typ = "plan.paused"
	}
	if err = m.appendLocked(liveJournal{Type: typ, PlanID: id}); err != nil {
		return LivePlanStatus{}, err
	}
	m.paused[id] = paused
	if !paused {
		go m.signal()
	}
	return m.statusLocked(id, plan), nil
}
func (m *LiveManager) statusLocked(id string, plan Run) LivePlanStatus {
	pending, err := m.pendingLocked(id)
	if err != nil {
		m.projectionDegraded = true
	}
	stat, _ := os.Stat(filepath.Join(m.root, "events.jsonl"))
	size := int64(0)
	if stat != nil {
		size = stat.Size()
	}
	return LivePlanStatus{ID: id, Name: id, Unit: plan.Effective.Plan.Stream.Unit, Paused: m.paused[id], Pending: pending, Storage: size, ProjectionDegraded: m.projectionDegraded}
}

var ErrLiveCapacity = errors.New("live query capacity exceeded")

func (m *LiveManager) Report(ctx context.Context, planID string, from, to time.Time) (LiveReport, error) {
	m.reportMu.Lock()
	defer m.reportMu.Unlock()
	base, err := m.plan(ctx, planID)
	if err != nil {
		return LiveReport{}, err
	}
	if base.Effective.Plan.Mode != "stream" {
		return LiveReport{}, errors.New("plan is not a stream")
	}
	if !to.After(from) || to.Sub(from) > 24*time.Hour {
		return LiveReport{}, errors.New("invalid report range")
	}
	m.mu.Lock()
	if err = m.ensureProjectionLocked(); err != nil {
		m.mu.Unlock()
		return LiveReport{}, err
	}
	windowSize := time.Duration(base.Effective.Plan.Stream.WindowSeconds) * time.Second
	alignedFrom, alignedTo := from.Truncate(windowSize), to.Truncate(windowSize).Add(windowSize)
	var count int
	err = m.db.QueryRow("SELECT count(*) FROM live_records WHERE plan=? AND expired=0 AND window_ns>=? AND window_ns<?", planID, alignedFrom.UnixNano(), alignedTo.UnixNano()).Scan(&count)
	if err != nil || count > MaxLiveRangeRecords {
		m.mu.Unlock()
		if err == nil {
			err = ErrLiveCapacity
		}
		return LiveReport{}, err
	}
	status := m.statusLocked(planID, base)
	snapshot := "report-" + Digest([]any{planID, m.seq, from, to})
	if err = m.snapshotRangeLocked(snapshot, planID, alignedFrom, alignedTo, false); err != nil {
		m.mu.Unlock()
		return LiveReport{}, err
	}
	defer func() {
		m.mu.Lock()
		_, _ = m.db.Exec("DELETE FROM live_snapshots WHERE id=?", snapshot)
		m.mu.Unlock()
	}()
	rows, err := m.db.Query("SELECT DISTINCT start_ns FROM live_snapshots WHERE id=? AND latest=1 AND window_ns>=? AND window_ns<? ORDER BY start_ns LIMIT ?", snapshot, from.UnixNano(), to.UnixNano(), MaxLiveReportWindows+1)
	starts := []time.Time{}
	if err == nil {
		for rows.Next() {
			var ns int64
			if err = rows.Scan(&ns); err != nil {
				break
			}
			starts = append(starts, time.Unix(0, ns).UTC())
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
	}
	out := LiveReport{Plan: status, From: from, To: to, Windows: []Run{}, Series: []LivePoint{}}
	if err == nil {
		err = m.db.QueryRow("SELECT count(*) FROM live_expired WHERE plan=? AND end_ns>? AND start_ns<?", planID, from.UnixNano(), to.UnixNano()).Scan(&out.ExpiredWindows)
	}
	m.mu.Unlock()
	if err != nil {
		return out, err
	}
	if len(starts) > MaxLiveReportWindows {
		return out, ErrLiveCapacity
	}
	viewBytes := 0
	for _, start := range starts {
		w, err := m.windowIndexed(ctx, base, start, snapshot)
		if err != nil {
			return out, err
		}
		view := runMetadata(w)
		view.Scope = cloneScope(w.Scope)
		view.Scope.Facts = nil
		view.Scope.BacklogFacts = nil
		encoded, encodeErr := json.Marshal(view)
		if encodeErr != nil {
			return out, encodeErr
		}
		viewBytes += len(encoded)
		if viewBytes > MaxMetadataBytes/2 {
			return LiveReport{}, ErrLiveCapacity
		}
		out.Windows = append(out.Windows, view)
		for _, o := range w.Outputs {
			if stats, ok := o.Data["window"].(map[string]any); ok {
				if series, ok := stats["series"]; ok {
					b, _ := json.Marshal(series)
					var point LivePoint
					if decodeJSON(b, &point) == nil {
						out.Series = append(out.Series, point)
					}
				}
			}
		}
	}
	if len(out.Windows) > 0 {
		current := out.Windows[len(out.Windows)-1]
		out.Current = &current
		if err = m.rangeSummary(ctx, base, &out, snapshot); err != nil {
			return out, err
		}
	}
	body, err := json.Marshal(out)
	if err != nil {
		return out, err
	}
	if len(body) > MaxMetadataBytes {
		return LiveReport{}, ErrLiveCapacity
	}
	return out, nil
}
func cloneScope(s *WindowScope) *WindowScope {
	if s == nil {
		return nil
	}
	c := *s
	return &c
}

func (m *LiveManager) windowIndexed(ctx context.Context, base Run, start time.Time, snapshot string) (Run, error) {
	end := start.Add(time.Duration(base.Effective.Plan.Stream.WindowSeconds) * time.Second)
	m.mu.Lock()
	if err := m.ensureProjectionLocked(); err != nil {
		m.mu.Unlock()
		return Run{}, err
	}
	count, revision, completed := 0, 1, 0
	err := m.db.QueryRow("SELECT count(*),coalesce(max(max(seq,scored_seq)),1),coalesce(sum(scored),0) FROM live_snapshots WHERE id=? AND latest=1 AND start_ns=?", snapshot, start.UnixNano()).Scan(&count, &revision, &completed)
	if err != nil || count > MaxLiveRangeRecords {
		m.mu.Unlock()
		if err == nil {
			err = ErrLiveCapacity
		}
		return Run{}, err
	}
	scope := &WindowScope{Kind: "window", PlanID: base.Effective.Plan.ID, Start: start, End: end, Watermark: end.Add(time.Duration(base.Effective.Plan.Stream.AllowedLatenessSeconds) * time.Second), Revision: revision, State: "provisional", Received: count, Eligible: count, Pending: count - completed, JudgeSampleRate: base.Effective.Plan.Stream.JudgeSampleRate, JudgeBudget: base.Effective.Plan.Stream.JudgePerWindowBudget}
	if !time.Now().UTC().Before(scope.Watermark) {
		scope.State = "final"
	}
	id := fmt.Sprintf("live-%s-%d-r%d-%s-%s", base.Effective.Plan.ID, start.Unix(), revision, scope.State, Digest(base.Effective)[:12])
	cached, getErr := m.Engine.Store.Get(id)
	if getErr == nil && cached.Message == "" {
		m.mu.Unlock()
		return cached, nil
	}
	if _, err = m.db.Exec("INSERT OR IGNORE INTO live_snapshots SELECT ?,seq,body,start_ns,window_ns,scored,scored_seq,latest FROM live_snapshots WHERE id=? AND start_ns=? AND latest=1", id, snapshot, start.UnixNano()); err != nil {
		m.mu.Unlock()
		return Run{}, err
	}
	m.mu.Unlock()
	defer func() { m.mu.Lock(); _, _ = m.db.Exec("DELETE FROM live_snapshots WHERE id=?", id); m.mu.Unlock() }()
	r := cloneLiveRun(base)
	r.ID = id
	r.CreatedAt = time.Now().UTC()
	r.Scope = scope
	r.Total = count
	r.Completed = completed
	r.Status = "completed"
	r.DatasetDigest = ""
	r.Cases = nil
	if count != completed || scope.State == "provisional" {
		r.Status = "partial"
	}
	scopeBytes := 0
	err = m.walkSnapshot(id, func(record liveRecord) error {
		fact := WindowFact{Origin: map[string]string{"source": record.Event.Source, "project": record.Event.Project, "trace_id": record.Event.TraceID, "turn_id": record.Event.TurnID, "event_id": record.Event.EventID, "case_id": record.Event.Case.ID}, CaseID: "live-" + Digest([]string{record.PlanID, record.Unit}), ReceivedAt: record.ReceivedAt, StartedAt: record.StartedAt, ScoredAt: record.ScoredAt, Revision: record.Event.EvidenceRevision}
		b, err := json.Marshal(m.Engine.redactForRun(&r, fact))
		if err != nil {
			return err
		}
		if err = decodeJSON(b, &fact); err != nil {
			return err
		}
		scopeBytes += len(b)
		if scopeBytes > MaxMetadataBytes/2 {
			return ErrLiveCapacity
		}
		scope.Facts = append(scope.Facts, fact)
		if record.Case != nil && record.Case.Status != "completed" {
			r.Status = "partial"
		}
		return nil
	})
	if err != nil {
		return r, err
	}
	r.caseSource = func(visit func(CaseResult) error) error {
		return m.walkSnapshot(id, func(record liveRecord) error {
			if record.Case != nil {
				return visit(*record.Case)
			}
			return nil
		})
	}
	r.inputSource = func(visit func(Case) error) error {
		return m.walkSnapshot(id, func(record liveRecord) error {
			c := record.Event.Case
			if record.Case != nil {
				c = record.Case.Case
			}
			c.ID = "live-" + Digest([]string{record.PlanID, record.Unit})
			b, err := json.Marshal(m.Engine.redactForRun(&r, c))
			if err != nil {
				return err
			}
			if err = decodeJSON(b, &c); err != nil {
				return err
			}
			return visit(c)
		})
	}
	if getErr != nil {
		r, err = m.Engine.Store.ImportRun(r)
		if err != nil {
			return r, err
		}
	} else {
		r.caseSource = cached.caseSource
		r.inputSource = cached.inputSource
	}
	if err = m.Engine.ProcessExisting(ctx, &r); err != nil {
		return r, err
	}
	for _, output := range r.Outputs {
		if stats, ok := output.Data["window"].(map[string]any); ok {
			b, _ := json.Marshal(stats)
			var summary WindowScope
			if decodeJSON(b, &summary) == nil {
				scope.Scored, scope.Sampled, scope.Failed, scope.NotComputable = summary.Scored, summary.Sampled, summary.Failed, summary.NotComputable
				scope.Coverage, scope.P50LatencyMS, scope.P95LatencyMS = summary.Coverage, summary.P50LatencyMS, summary.P95LatencyMS
				scope.Reasons, scope.Anomalies = summary.Reasons, summary.Anomalies
				scope.Skipped, scope.BudgetExhausted, scope.Unscored = summary.Skipped, summary.BudgetExhausted, summary.Unscored
				scope.StatisticsAvailable = true
			}
		}
	}
	r.Scope = scope
	if err = m.Engine.Store.Commit(&r, "window.materialized", nil); err != nil {
		return r, err
	}
	view := runMetadata(r)
	if err = atomicJSON(filepath.Join(m.root, "windows", Digest(id)+".json"), view); err != nil {
		return r, err
	}
	return m.Engine.Store.Get(id)
}
func (m *LiveManager) Window(ctx context.Context, id string) (Run, error) {
	m.mu.Lock()
	expired, projectionErr := m.expiredWindowLocked(id)
	m.mu.Unlock()
	if projectionErr != nil {
		return Run{}, projectionErr
	}
	if expired {
		return Run{}, ErrDataExpired
	}
	r, err := m.Engine.Store.Get(id)
	if err != nil || r.Scope == nil {
		return Run{}, errors.New("window not found")
	}
	return r, nil
}

// MapOTLPJSON accepts the deliberately documented OpenInference JSON subset.
func MapOTLPJSON(body []byte) (IngestBatch, error) {
	var raw otlpRequest
	if err := decodeJSON(body, &raw); err != nil {
		return IngestBatch{}, err
	}
	batch := IngestBatch{}
	for _, rs := range raw.ResourceSpans {
		resource := attrs(rs.Resource.Attributes)
		planID := resource["seven.eval.plan.id"]
		if planID == "" {
			return IngestBatch{}, errors.New("OTLP plan mapping missing")
		}
		if batch.PlanID != "" && batch.PlanID != planID {
			return IngestBatch{}, errors.New("OTLP batch contains multiple plans")
		}
		batch.PlanID = planID
		source := resource["service.name"]
		if source == "" {
			source = "otlp"
		}
		project := resource["seven.eval.project"]
		if project == "" {
			project = source
		}
		for _, ss := range rs.ScopeSpans {
			byTrace := map[string][]otlpSpan{}
			for _, span := range ss.Spans {
				byTrace[span.TraceID] = append(byTrace[span.TraceID], span)
			}
			for trace, spans := range byTrace {
				var root *otlpSpan
				for i := range spans {
					a := attrs(spans[i].Attributes)
					if spans[i].ParentSpanID == "" {
						root = &spans[i]
						break
					}
					if root == nil && (a["openinference.span.kind"] == "AGENT" || a["openinference.span.kind"] == "CHAIN") {
						root = &spans[i]
					}
				}
				if root == nil {
					return IngestBatch{}, errors.New("OTLP root mapping missing")
				}
				if root.EndTimeUnixNano == 0 {
					return IngestBatch{}, errors.New("OTLP root mapping missing")
				}
				a := attrs(root.Attributes)
				contexts := []any{}
				for _, span := range spans {
					sa := attrs(span.Attributes)
					if sa["openinference.span.kind"] == "RETRIEVER" {
						keys := make([]string, 0, len(sa))
						for key := range sa {
							keys = append(keys, key)
						}
						sort.Strings(keys)
						for _, key := range keys {
							if strings.HasPrefix(key, "retrieval.documents.") && strings.HasSuffix(key, ".document.content") {
								contexts = append(contexts, sa[key])
							}
						}
					}
				}
				latency := float64(0)
				if root.EndTimeUnixNano > root.StartTimeUnixNano {
					latency = float64(root.EndTimeUnixNano-root.StartTimeUnixNano) / 1e6
				}
				turn := a["seven.eval.turn.id"]
				if turn == "" {
					turn = resource["seven.eval.turn.id"]
				}
				usage := map[string]any{}
				// Prefer a root aggregate; otherwise sum completed LLM child facts.
				for attribute, key := range map[string]string{"llm.token_count.total": "total_tokens", "llm.token_count.prompt": "input_tokens", "llm.token_count.completion": "output_tokens"} {
					if value, err := strconv.ParseFloat(a[attribute], 64); err == nil && value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0) {
						usage[key] = value
						continue
					}
					total, found := 0.0, false
					for _, span := range spans {
						child := attrs(span.Attributes)
						if child["openinference.span.kind"] == "LLM" {
							if value, err := strconv.ParseFloat(child[attribute], 64); err == nil && value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0) {
								total += value
								found = true
							}
						}
					}
					if found {
						usage[key] = total
					}
				}
				metadata := map[string]any{}
				if a["llm.model_name"] != "" {
					metadata["openinference.model"] = a["llm.model_name"]
				}
				batch.Events = append(batch.Events, ProductionEvent{SchemaVersion: "evaluate.event/v1", Source: source, Project: project, EventID: "otlp:" + trace, EventTime: time.Unix(0, int64(root.EndTimeUnixNano)).UTC(), TraceID: trace, TurnID: turn, EvidenceRevision: 1, Case: Case{ID: trace, Inputs: decodeOTLPValue(a["input.value"], a["input.mime_type"]), Metadata: metadata}, Result: SubjectResult{Response: decodeOTLPValue(a["output.value"], a["output.mime_type"]), Contexts: contexts, LatencyMS: &latency, Usage: usage}})
			}
		}
	}
	if batch.PlanID == "" || len(batch.Events) == 0 {
		return IngestBatch{}, errors.New("OTLP contains no mappable trace")
	}
	return batch, nil
}

type otlpRequest struct {
	ResourceSpans []otlpResourceSpan `json:"resourceSpans"`
}
type otlpResourceSpan struct {
	Resource struct {
		Attributes []otlpAttr `json:"attributes"`
	} `json:"resource"`
	ScopeSpans []struct {
		Spans []otlpSpan `json:"spans"`
	} `json:"scopeSpans"`
}
type otlpAttr struct {
	Key   string `json:"key"`
	Value struct {
		StringValue string      `json:"stringValue"`
		IntValue    json.Number `json:"intValue,omitempty"`
		DoubleValue *float64    `json:"doubleValue,omitempty"`
	} `json:"value"`
}
type otlpSpan struct {
	TraceID           string     `json:"traceId"`
	SpanID            string     `json:"spanId"`
	ParentSpanID      string     `json:"parentSpanId"`
	StartTimeUnixNano uint64     `json:"startTimeUnixNano,string"`
	EndTimeUnixNano   uint64     `json:"endTimeUnixNano,string"`
	Attributes        []otlpAttr `json:"attributes"`
}

func attrs(values []otlpAttr) map[string]string {
	out := map[string]string{}
	for _, v := range values {
		out[v.Key] = v.Value.StringValue
		if v.Value.IntValue != "" {
			out[v.Key] = v.Value.IntValue.String()
		}
		if v.Value.DoubleValue != nil {
			out[v.Key] = strconv.FormatFloat(*v.Value.DoubleValue, 'f', -1, 64)
		}
	}
	return out
}
func decodeOTLPValue(value, mime string) any {
	if mime == "application/json" {
		var parsed any
		if decodeJSON([]byte(value), &parsed) == nil {
			return parsed
		}
	}
	return value
}

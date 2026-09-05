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
	"math"
	"os"
	"path/filepath"
	"seven-agent-eval/protocol"
	"sort"
	"strings"
	"sync"
	"time"
)

type liveRecord struct {
	PlanID     string          `json:"plan_id"`
	Event      ProductionEvent `json:"event"`
	Digest     string          `json:"digest"`
	ReceivedAt time.Time       `json:"received_at"`
	ScoredAt   *time.Time      `json:"scored_at,omitempty"`
	Case       *CaseResult     `json:"case,omitempty"`
}
type liveJournal struct {
	Sequence int         `json:"sequence"`
	Type     string      `json:"type"`
	Time     time.Time   `json:"time"`
	PlanID   string      `json:"plan_id,omitempty"`
	Record   *liveRecord `json:"record,omitempty"`
}
type LivePlanStatus struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Unit    string `json:"unit"`
	Paused  bool   `json:"paused"`
	Pending int    `json:"pending"`
	Storage int64  `json:"storage_bytes"`
}
type LivePoint struct {
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
	Plan    LivePlanStatus `json:"plan"`
	From    time.Time      `json:"from"`
	To      time.Time      `json:"to"`
	Current *Run           `json:"current,omitempty"`
	Windows []Run          `json:"windows"`
	Series  []LivePoint    `json:"series"`
}
type LiveManager struct {
	Engine  *Engine
	root    string
	db      *sql.DB
	mu      sync.Mutex
	seq     int
	records map[string]*liveRecord // plan + source + event id
	plans   map[string]Run
	paused  map[string]bool
	wake    chan struct{}
	stop    context.CancelFunc
	wg      sync.WaitGroup
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
	if _, err = db.Exec("CREATE TABLE IF NOT EXISTS live_projection (id TEXT PRIMARY KEY, body BLOB NOT NULL)"); err != nil {
		db.Close()
		return nil, err
	}
	m := &LiveManager{Engine: engine, root: root, db: db, records: map[string]*liveRecord{}, plans: map[string]Run{}, paused: map[string]bool{}, wake: make(chan struct{}, 1)}
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
	return out
}

func unitKey(plan Plan, e ProductionEvent) (string, error) {
	if plan.Stream == nil {
		return "", errors.New("stream configuration missing")
	}
	if plan.Stream.Unit == "trace" && e.TraceID != "" {
		return "trace:" + e.Source + ":" + e.TraceID, nil
	}
	if plan.Stream.Unit == "turn" && e.TurnID != "" {
		return "turn:" + e.Source + ":" + e.TurnID, nil
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
	m.plans[id] = r
	m.mu.Unlock()
	return cloneLiveRun(r), nil
}
func (m *LiveManager) appendLocked(ev liveJournal) error {
	ev.Sequence = m.seq + 1
	ev.Time = time.Now().UTC()
	b, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(m.root, "events.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	m.seq = ev.Sequence
	// The journal is authoritative. This SQLite write is only a query projection.
	if _, err = m.db.Exec("INSERT INTO live_projection(id,body) VALUES(?,?) ON CONFLICT(id) DO UPDATE SET body=excluded.body", fmt.Sprintf("%09d", ev.Sequence), b); err != nil {
		return &ProjectionError{Err: err}
	}
	return nil
}
func (m *LiveManager) rebuild() error {
	if _, err := m.db.Exec("DELETE FROM live_projection"); err != nil {
		return err
	}
	p := filepath.Join(m.root, "events.jsonl")
	f, err := os.Open(p)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 4096), 2<<20)
	for s.Scan() {
		var ev liveJournal
		if err := decodeJSON(s.Bytes(), &ev); err != nil {
			return errors.New("corrupt live journal")
		}
		if ev.Sequence != m.seq+1 {
			return errors.New("live journal sequence gap")
		}
		m.seq = ev.Sequence
		if _, err := m.db.Exec("INSERT INTO live_projection(id,body) VALUES(?,?)", fmt.Sprintf("%09d", ev.Sequence), s.Bytes()); err != nil {
			return err
		}
		switch ev.Type {
		case "event.accepted":
			if ev.Record != nil {
				copy := *ev.Record
				m.records[liveKey(copy.PlanID, copy.Event.Source, copy.Event.EventID)] = &copy
			}
		case "event.scored":
			if ev.Record != nil {
				if old := m.records[liveKey(ev.Record.PlanID, ev.Record.Event.Source, ev.Record.Event.EventID)]; old != nil {
					old.Case, old.ScoredAt = ev.Record.Case, ev.Record.ScoredAt
				}
			}
		case "plan.paused":
			m.paused[ev.PlanID] = true
		case "plan.resumed":
			m.paused[ev.PlanID] = false
		}
	}
	return s.Err()
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
	pending := 0
	for _, r := range m.records {
		if r.PlanID == batch.PlanID && r.Case == nil {
			pending++
		}
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
		if prior := m.records[key]; prior != nil {
			if prior.Digest == digest {
				out.Duplicate++
				out.Items = append(out.Items, IngestItem{EventID: event.EventID, Status: "duplicate"})
			} else {
				out.Rejected++
				out.Items = append(out.Items, IngestItem{EventID: event.EventID, Status: "rejected", Code: "EVENT_CONFLICT"})
			}
			continue
		}
		if pending >= planRun.Effective.Plan.Stream.MaxPending {
			out.Rejected++
			out.Items = append(out.Items, IngestItem{EventID: event.EventID, Status: "rejected", Code: "QUEUE_FULL"})
			continue
		}
		record := &liveRecord{PlanID: batch.PlanID, Event: event, Digest: digest, ReceivedAt: time.Now().UTC()}
		if err := m.appendLocked(liveJournal{Type: "event.accepted", PlanID: batch.PlanID, Record: record}); err != nil {
			return out, err
		}
		m.records[key] = record
		pending++
		out.Accepted++
		out.Items = append(out.Items, IngestItem{EventID: event.EventID, Status: "accepted"})
	}
	go m.signal()
	return out, nil
}
func (m *LiveManager) scorePending(ctx context.Context) {
	m.mu.Lock()
	work := []*liveRecord{}
	for _, r := range m.records {
		if r.Case == nil && !m.paused[r.PlanID] {
			copy := *r
			work = append(work, &copy)
		}
	}
	m.mu.Unlock()
	for _, record := range work {
		plan, err := m.plan(ctx, record.PlanID)
		if err != nil {
			continue
		}
		cctx, cancel := context.WithTimeout(ctx, time.Duration(plan.Effective.Plan.TimeoutSeconds)*time.Second)
		caseResult, err := m.Engine.EvaluateExisting(cctx, &plan, record.Event.Case, record.Event.Result)
		cancel()
		if err != nil {
			continue
		}
		now := time.Now().UTC()
		record.Case, record.ScoredAt = &caseResult, &now
		m.mu.Lock()
		original := m.records[liveKey(record.PlanID, record.Event.Source, record.Event.EventID)]
		if original != nil && original.Case == nil {
			if m.appendLocked(liveJournal{Type: "event.scored", PlanID: record.PlanID, Record: record}) == nil {
				original.Case, original.ScoredAt = record.Case, record.ScoredAt
			}
		}
		m.mu.Unlock()
		m.signal()
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
	pending := 0
	for _, r := range m.records {
		if r.PlanID == id && r.Case == nil {
			pending++
		}
	}
	stat, _ := os.Stat(filepath.Join(m.root, "events.jsonl"))
	size := int64(0)
	if stat != nil {
		size = stat.Size()
	}
	return LivePlanStatus{ID: id, Name: id, Unit: plan.Effective.Plan.Stream.Unit, Paused: m.paused[id], Pending: pending, Storage: size}
}
func (m *LiveManager) Report(ctx context.Context, planID string, from, to time.Time) (LiveReport, error) {
	plan, err := m.plan(ctx, planID)
	if err != nil {
		return LiveReport{}, err
	}
	if plan.Effective.Plan.Mode != "stream" {
		return LiveReport{}, errors.New("plan is not a stream")
	}
	if to.Before(from) || to.Equal(from) {
		return LiveReport{}, errors.New("invalid report range")
	}
	m.mu.Lock()
	records := []liveRecord{}
	for _, r := range m.records {
		if r.PlanID == planID {
			records = append(records, *r)
		}
	}
	status := m.statusLocked(planID, plan)
	m.mu.Unlock()
	latest := map[string]liveRecord{}
	for _, r := range records {
		unit, _ := unitKey(plan.Effective.Plan, r.Event)
		if old, ok := latest[unit]; !ok || r.Event.EvidenceRevision > old.Event.EvidenceRevision || (r.Event.EvidenceRevision == old.Event.EvidenceRevision && r.ReceivedAt.After(old.ReceivedAt)) {
			latest[unit] = r
		}
	}
	groups := map[time.Time][]liveRecord{}
	step := time.Duration(plan.Effective.Plan.Stream.WindowSeconds) * time.Second
	for _, r := range latest {
		if r.Event.EventTime.Before(from) || !r.Event.EventTime.Before(to) {
			continue
		}
		start := r.Event.EventTime.UTC().Truncate(step)
		groups[start] = append(groups[start], r)
	}
	starts := make([]time.Time, 0, len(groups))
	for start := range groups {
		starts = append(starts, start)
	}
	sort.Slice(starts, func(i, j int) bool { return starts[i].Before(starts[j]) })
	out := LiveReport{Plan: status, From: from, To: to, Windows: []Run{}, Series: []LivePoint{}}
	for _, start := range starts {
		window := m.window(ctx, plan, start, groups[start])
		out.Windows = append(out.Windows, window)
		scope := window.Scope
		coverage := (*float64)(nil)
		if scope.Eligible > 0 {
			v := float64(scope.Scored) / float64(scope.Eligible)
			coverage = &v
		}
		out.Series = append(out.Series, LivePoint{Time: start, Coverage: coverage, Received: scope.Received, Scored: scope.Scored, NotComputable: scope.NotComputable, Failed: scope.Failed, Pending: scope.Pending, P50LatencyMS: scope.P50LatencyMS, P95LatencyMS: scope.P95LatencyMS})
	}
	if len(out.Windows) > 0 {
		current := out.Windows[len(out.Windows)-1]
		out.Current = &current
	}
	return out, nil
}
func (m *LiveManager) window(ctx context.Context, base Run, start time.Time, records []liveRecord) Run {
	end := start.Add(time.Duration(base.Effective.Plan.Stream.WindowSeconds) * time.Second)
	scope := &WindowScope{Kind: "window", PlanID: base.Effective.Plan.ID, Start: start, End: end, Watermark: end.Add(time.Duration(base.Effective.Plan.Stream.AllowedLatenessSeconds) * time.Second), Revision: 1, State: "provisional", Received: len(records), Eligible: len(records), JudgeSampleRate: base.Effective.Plan.Stream.JudgeSampleRate, JudgeBudget: base.Effective.Plan.Stream.JudgePerWindowBudget}
	if !time.Now().UTC().Before(scope.Watermark) {
		scope.State = "final"
	}
	latencies := []float64{}
	r := cloneLiveRun(base)
	r.ID = fmt.Sprintf("live-%s-%d-r1", base.Effective.Plan.ID, start.Unix())
	r.CreatedAt = time.Now().UTC()
	r.Total = len(records)
	r.Completed = 0
	r.Cases = []CaseResult{}
	r.Scope = scope
	r.DatasetDigest = ""
	r.Status = "completed"
	for _, record := range records {
		if record.Event.EvidenceRevision > scope.Revision {
			scope.Revision = record.Event.EvidenceRevision
			r.ID = fmt.Sprintf("live-%s-%d-r%d", base.Effective.Plan.ID, start.Unix(), scope.Revision)
		}
		if record.Case == nil {
			scope.Pending++
			r.Status = "partial"
			continue
		}
		r.Cases = append(r.Cases, *record.Case)
		r.Completed++
		if record.Case.Status == "error" {
			scope.Failed++
		}
		observed := false
		notComputable := false
		evaluationFailed := false
		for _, m := range record.Case.Metrics {
			if m.Status == "observed" {
				observed = true
			}
			if m.Status == "not_computable" {
				notComputable = true
			}
			if m.Status == "error" {
				evaluationFailed = true
			}
		}
		if observed {
			scope.Scored++
		}
		if evaluationFailed {
			scope.Failed++
		} else if notComputable && !observed {
			scope.NotComputable++
		}
		if record.Event.Result.LatencyMS != nil {
			latencies = append(latencies, *record.Event.Result.LatencyMS)
		}
	}
	scope.Sampled = scope.Scored
	scope.P50LatencyMS = percentile(latencies, .5)
	scope.P95LatencyMS = percentile(latencies, .95)
	if scope.State == "provisional" {
		r.Status = "partial"
	}
	_ = m.Engine.ProcessExisting(ctx, &r)
	if b, err := json.Marshal(m.Engine.redactForRun(&r, r)); err == nil {
		_ = decodeJSON(b, &r)
	}
	return r
}
func (m *LiveManager) Window(ctx context.Context, id string) (Run, error) {
	now := time.Now().UTC()
	reports, err := m.Plans(ctx)
	if err != nil {
		return Run{}, err
	}
	for _, p := range reports {
		r, e := m.Report(ctx, p.ID, now.Add(-30*24*time.Hour), now.Add(time.Minute))
		if e != nil {
			continue
		}
		for _, w := range r.Windows {
			if w.ID == id {
				return w, nil
			}
		}
	}
	return Run{}, errors.New("window not found")
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
					if spans[i].ParentSpanID == "" || a["openinference.span.kind"] == "AGENT" || a["openinference.span.kind"] == "CHAIN" {
						root = &spans[i]
						break
					}
				}
				if root == nil {
					return IngestBatch{}, errors.New("OTLP root mapping missing")
				}
				a := attrs(root.Attributes)
				contexts := []any{}
				for _, span := range spans {
					sa := attrs(span.Attributes)
					if sa["openinference.span.kind"] == "RETRIEVER" {
						for key, value := range sa {
							if strings.HasPrefix(key, "retrieval.documents.") && strings.HasSuffix(key, ".document.content") {
								contexts = append(contexts, value)
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
				batch.Events = append(batch.Events, ProductionEvent{SchemaVersion: "evaluate.event/v1", Source: source, Project: project, EventID: "otlp:" + trace, EventTime: time.Unix(0, int64(root.EndTimeUnixNano)).UTC(), TraceID: trace, TurnID: turn, EvidenceRevision: 1, Case: Case{ID: trace, Inputs: decodeOTLPValue(a["input.value"], a["input.mime_type"])}, Result: SubjectResult{Response: decodeOTLPValue(a["output.value"], a["output.mime_type"]), Contexts: contexts, LatencyMS: &latency}})
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
		StringValue string `json:"stringValue"`
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

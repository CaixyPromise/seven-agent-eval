package core

// Retention is deliberately explicit, quiescent maintenance. It is not a daemon
// timer and never executes user extensions. The Store owner lock excludes servers.
import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var ErrDataExpired = errors.New("evaluation data expired")

type ExpiredWindow struct {
	PlanID string    `json:"plan_id"`
	Start  time.Time `json:"start"`
	End    time.Time `json:"end"`
}

func (m *LiveManager) expiredEventLocked(planID string, at time.Time) (bool, error) {
	if err := m.ensureProjectionLocked(); err != nil {
		return false, err
	}
	var count int
	err := m.db.QueryRow("SELECT count(*) FROM live_expired WHERE plan=? AND start_ns<=? AND end_ns>?", planID, at.UnixNano(), at.UnixNano()).Scan(&count)
	return count > 0, err
}

type RetentionPlan struct {
	Version     string    `json:"version"`
	Before      time.Time `json:"before"`
	StateDigest string    `json:"state_digest"`
	Digest      string    `json:"digest"`
	Records     []string  `json:"records"`
	Assemblies  []string  `json:"assemblies"`
	Windows     []string  `json:"windows"`
	Runs        []string  `json:"runs"`
	Protected   int       `json:"protected"`
}
type RetentionAudit struct {
	Digest    string    `json:"digest"`
	Before    time.Time `json:"before"`
	AppliedAt time.Time `json:"applied_at"`
	Records   int       `json:"records"`
	Windows   int       `json:"windows"`
	Runs      int       `json:"runs"`
}

func (s *Store) expiredRun(id string) bool {
	if !validID.MatchString(id) {
		return false
	}
	_, err := os.Stat(filepath.Join(s.Root, "retention", "expired-runs", id+".json"))
	return err == nil
}

// Only authoritative data participates: opening/rebuilding SQLite must not make
// a preview stale. Source/runtime files are included to fence changed references.
func retentionState(root string) (string, error) {
	hashes := map[string]string{}
	for _, rel := range []string{"runs", "live/events.jsonl", "live/windows", "retention/expired-runs"} {
		base := filepath.Join(root, rel)
		err := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
			if os.IsNotExist(err) && path == base {
				return nil
			}
			if err != nil {
				return err
			}
			if d.Type()&os.ModeSymlink != 0 {
				return errors.New("retention refuses symlinks")
			}
			if d.IsDir() {
				return nil
			}
			if !d.Type().IsRegular() {
				return errors.New("retention refuses special files")
			}
			if d.Name() == "result.json" {
				return nil
			} // rebuildable projection
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			h := sha256.New()
			_, err = io.Copy(h, f)
			ce := f.Close()
			if err != nil {
				return err
			}
			if ce != nil {
				return ce
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			hashes[relative] = hex.EncodeToString(h.Sum(nil))
			return nil
		})
		if err != nil {
			return "", err
		}
	}
	return Digest(hashes), nil
}

// Preview expires only materialized final windows. Unmaterialized evidence,
// unfinished windows, orphan snapshots and partial offline runs stay protected.
func (m *LiveManager) PreviewRetention(before time.Time) (RetentionPlan, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := RetentionPlan{Version: "1", Before: before.UTC(), Records: []string{}, Assemblies: []string{}, Windows: []string{}, Runs: []string{}}
	if m.stop != nil || len(m.plans) != 0 {
		return p, errors.New("retention requires a fresh stopped maintenance manager")
	}
	if before.IsZero() || before.After(time.Now()) {
		return p, errors.New("retention cutoff must be an explicit past time")
	}
	digest, err := retentionState(m.Engine.Store.Root)
	if err != nil {
		return p, err
	}
	records, assemblies, _, _, err := m.retentionViewLocked()
	if err != nil {
		return p, err
	}
	p.StateDigest = digest
	windows := []Run{}
	entries, err := os.ReadDir(filepath.Join(m.root, "windows"))
	if err != nil && !os.IsNotExist(err) {
		return p, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		body, err := ReadBounded(filepath.Join(m.root, "windows", entry.Name()), MaxMetadataBytes)
		if err != nil {
			return p, err
		}
		var w Run
		if err = decodeJSON(body, &w); err != nil {
			return p, err
		}
		if w.Scope == nil || entry.Name() != Digest(w.ID)+".json" {
			return p, errors.New("invalid window snapshot")
		}
		w.Scope.Facts, w.Scope.BacklogFacts = nil, nil
		w.Cases, w.Outputs, w.Conclusions, w.Catalogs = nil, nil, nil, nil
		if len(windows) >= MaxRetentionIdentities {
			return p, errors.New("retention window limit exceeded")
		}
		windows = append(windows, w)
	}
	selected := map[string]bool{}
	inWindow := func(r *liveRecord, w Run) bool {
		return r.PlanID == w.Scope.PlanID && !r.WindowTime.Before(w.Scope.Start) && r.WindowTime.Before(w.Scope.End)
	}
	for _, w := range windows {
		if w.Scope.State != "final" || !w.Scope.Watermark.Before(before) {
			continue
		}
		safe := true
		for _, r := range records {
			if inWindow(r, w) && !r.Expired && (r.Case == nil || r.ScoredAt == nil || !r.ScoredAt.Before(before)) {
				safe = false
				break
			}
		}
		if safe {
			for key, r := range records {
				if inWindow(r, w) && !r.Expired {
					selected[key] = true
				}
			}
		}
	}
	refs := map[string]bool{}
	protectRef := func(r Run) {
		if r.Effective.NodeRuntime != nil {
			refs[r.Effective.NodeRuntime.SnapshotID] = true
		}
	}
	for _, w := range windows {
		eligible := w.Scope.Watermark.Before(before)
		for key, r := range records {
			if inWindow(r, w) && !r.Expired && !selected[key] {
				eligible = false
			}
		}
		if eligible {
			p.Windows = append(p.Windows, w.ID)
		} else {
			protectRef(w)
		}
	}
	for key, r := range records {
		if selected[key] {
			p.Records = append(p.Records, key)
		} else if !r.Expired {
			p.Protected++
		}
	}
	for key, a := range assemblies {
		if a.ExpiredSpans != nil {
			continue
		}
		source := attrs(a.Resource)["service.name"]
		if source == "" {
			source = "otlp"
		}
		recordKey := liveKey(a.PlanID, source, "otlp:"+a.Key+":"+a.ContentDigest)
		if selected[recordKey] && a.UpdatedAt.Before(before) {
			p.Assemblies = append(p.Assemblies, key)
		} else {
			p.Protected++
		}
	}
	runs, err := m.Engine.Store.List()
	if err != nil {
		return p, err
	}
	candidates := map[string]bool{}
	selectedWindows := map[string]bool{}
	for _, id := range p.Windows {
		selectedWindows[id] = true
	}
	for _, r := range runs {
		st, err := os.Stat(filepath.Join(m.Engine.Store.Root, "runs", r.ID, "events.jsonl"))
		if err != nil {
			return p, err
		}
		eligible := (r.Status == "completed" || r.Status == "failed" || r.Status == "cancelled") && st.ModTime().Before(before)
		if r.Scope != nil {
			eligible = selectedWindows[r.ID]
		}
		if eligible {
			candidates[r.ID] = true
		} else {
			protectRef(r)
			p.Protected++
		}
	}
	// References may transitively protect a candidate's own runtime snapshot.
	for changed := true; changed; {
		changed = false
		for _, r := range runs {
			if candidates[r.ID] && refs[r.ID] {
				delete(candidates, r.ID)
				protectRef(r)
				p.Protected++
				changed = true
			}
		}
	}
	for id := range candidates {
		p.Runs = append(p.Runs, id)
	}
	// A previous explicit apply may have published the receipt before crashing
	// during removal. Such a directory cannot be resumed or resurrected.
	dirs, err := os.ReadDir(filepath.Join(m.Engine.Store.Root, "runs"))
	if err != nil {
		return p, err
	}
	for _, dir := range dirs {
		if dir.IsDir() && validID.MatchString(dir.Name()) && m.Engine.Store.expiredRun(dir.Name()) {
			p.Runs = append(p.Runs, dir.Name())
		}
	}
	for _, list := range [][]string{p.Records, p.Assemblies, p.Windows, p.Runs} {
		sort.Strings(list)
	}
	p.Digest = Digest(p)
	return p, nil
}

func (m *LiveManager) SaveRetentionPreview(p RetentionPlan) error {
	if p.Digest == "" {
		return errors.New("missing preview digest")
	}
	return atomicJSON(filepath.Join(m.Engine.Store.Root, "retention", "previews", p.Digest+".json"), p)
}

func (m *LiveManager) ApplyRetention(ctx context.Context, id string) (RetentionAudit, error) {
	var audit RetentionAudit
	if len(id) != 64 {
		return audit, errors.New("invalid preview digest")
	}
	if _, err := hex.DecodeString(id); err != nil {
		return audit, errors.New("invalid preview digest")
	}
	data, err := ReadBounded(filepath.Join(m.Engine.Store.Root, "retention", "previews", id+".json"), 64<<20)
	if err != nil {
		return audit, err
	}
	var p RetentionPlan
	if err = decodeJSON(data, &p); err != nil {
		return audit, err
	}
	current, err := m.PreviewRetention(p.Before)
	if err != nil {
		return audit, err
	}
	if current.Digest != id || p.Digest != id {
		return audit, errors.New("retention preview stale; preview again")
	}
	p = current // never trust edited target lists in a saved preview
	m.mu.Lock()
	defer m.mu.Unlock()
	if ctx.Err() != nil {
		return audit, ctx.Err()
	}
	audit = RetentionAudit{Digest: id, Before: p.Before, AppliedAt: time.Now().UTC(), Records: len(p.Records), Windows: len(p.Windows), Runs: len(p.Runs)}
	if err = m.compactRetention(ctx, p, audit); err != nil {
		return audit, err
	}
	// Published expiration receipts are authoritative even if physical deletion
	// is interrupted. A new preview safely retries remaining file reclamation.
	for _, window := range p.Windows {
		path, err := SafePath(m.root, filepath.Join("windows", Digest(window)+".json"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return audit, err
		}
		if err = os.Remove(path); err != nil {
			return audit, err
		}
	}
	if len(p.Windows) > 0 {
		if err = syncDirectory(filepath.Join(m.root, "windows")); err != nil {
			return audit, err
		}
	}
	for _, run := range p.Runs {
		if !validID.MatchString(run) {
			return audit, errors.New("invalid retention run id")
		}
		path, err := SafePath(m.Engine.Store.Root, filepath.Join("runs", run))
		if err != nil {
			return audit, err
		}
		if err = atomicJSON(filepath.Join(m.Engine.Store.Root, "retention", "expired-runs", run+".json"), audit); err != nil {
			return audit, err
		}
		if err = os.RemoveAll(path); err != nil {
			return audit, err
		}
		if _, err = m.Engine.Store.db.Exec("DELETE FROM runs WHERE id=?", run); err != nil {
			return audit, err
		}
		if _, err = m.Engine.Store.db.Exec("DELETE FROM run_cases WHERE run_id=?", run); err != nil {
			return audit, err
		}
	}
	if len(p.Runs) > 0 {
		if err = syncDirectory(filepath.Join(m.Engine.Store.Root, "runs")); err != nil {
			return audit, err
		}
	}
	if err = atomicJSON(filepath.Join(m.Engine.Store.Root, "retention", "audit", id+".json"), audit); err != nil {
		return audit, err
	}
	// Queries are projections; VACUUM reclaims deleted evidence in SQLite too.
	if _, err = m.db.Exec("VACUUM"); err != nil {
		return audit, err
	}
	if _, err = m.Engine.Store.db.Exec("VACUUM"); err != nil {
		return audit, err
	}
	return audit, nil
}

func (m *LiveManager) compactRetention(ctx context.Context, p RetentionPlan, audit RetentionAudit) error {
	selected, assemblies := map[string]bool{}, map[string]bool{}
	for _, key := range p.Records {
		selected[key] = true
	}
	for _, key := range p.Assemblies {
		assemblies[key] = true
	}
	f, err := os.CreateTemp(m.root, ".compact-")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	defer f.Close()
	_, _, expiredRanges, retentionAudits, err := m.retentionViewLocked()
	if err != nil {
		return err
	}
	seq := m.seq
	write := func(ev liveJournal) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		seq++
		ev.Sequence = seq
		ev.Time = audit.AppliedAt
		b, e := json.Marshal(ev)
		if e != nil {
			return e
		}
		if len(b)+1 > 64<<20 {
			return errors.New("compacted record too large")
		}
		_, e = f.Write(append(b, '\n'))
		return e
	}
	if err = write(liveJournal{Type: "journal.compacted"}); err != nil {
		return err
	}
	err = m.walkRecordsLocked(func(key string, record *liveRecord) error {
		r := *record
		if selected[key] {
			r.Expired = true
			r.Case = nil
			r.Event.Case = Case{}
			r.Event.Result = SubjectResult{}
		}
		return write(liveJournal{Type: "event.restored", Record: &r})
	})
	if err != nil {
		return err
	}
	err = m.walkAssembliesLocked(func(key string, a otlpAssembly) error {
		if assemblies[key] {
			a.ExpiredSpans = map[string]string{}
			for _, span := range a.Spans {
				a.ExpiredSpans[span.SpanID] = Digest(span)
			}
			a.Spans = nil
			a.Resource = nil
		}
		return write(liveJournal{Type: "otlp.updated", Assembly: &a})
	})
	if err != nil {
		return err
	}
	for id, paused := range m.paused {
		typ := "plan.resumed"
		if paused {
			typ = "plan.paused"
		}
		if err = write(liveJournal{Type: typ, PlanID: id}); err != nil {
			return err
		}
	}
	expiredWindows := map[string]bool{}
	for id := range expiredRanges {
		expiredWindows[id] = true
	}
	for _, id := range p.Windows {
		expiredWindows[id] = true
	}
	for id := range expiredWindows {
		scope, ok := expiredRanges[id]
		if !ok {
			body, e := ReadBounded(filepath.Join(m.root, "windows", Digest(id)+".json"), 64<<20)
			if e != nil {
				return e
			}
			var w Run
			if e = decodeJSON(body, &w); e != nil {
				return e
			}
			if w.Scope == nil {
				return errors.New("missing expiry scope")
			}
			scope = ExpiredWindow{PlanID: w.Scope.PlanID, Start: w.Scope.Start, End: w.Scope.End}
		}
		if err = write(liveJournal{Type: "window.expired", WindowID: id, ExpiredScope: &scope}); err != nil {
			return err
		}
	}
	for _, old := range retentionAudits {
		if err = write(liveJournal{Type: "retention.applied", Retention: &old}); err != nil {
			return err
		}
	}
	if err = write(liveJournal{Type: "retention.applied", Retention: &audit}); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	publish := m.compactPublish
	if publish == nil {
		publish = os.Rename
	}
	if err = publish(name, filepath.Join(m.root, "events.jsonl")); err != nil {
		return err
	}
	if err = syncDirectory(m.root); err != nil {
		m.journalFailure = err
		return err
	}
	m.paused = map[string]bool{}
	m.seq = 0
	if err = m.rebuild(); err != nil {
		m.journalFailure = err
		return fmt.Errorf("retention committed; reopen to rebuild: %w", err)
	}
	return nil
}

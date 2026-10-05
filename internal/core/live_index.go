package core

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"
)

const LiveReadBatch = 32
const MaxLivePlans = 128
const MaxLiveReportWindows = 1440
const MaxLiveRangeRecords = 10000
const MaxRetentionIdentities = 50000

type liveSQL interface {
	Exec(string, ...any) (sql.Result, error)
}

// These are disposable indexes. The synced journal is the recovery authority.
const liveDDL = `
PRAGMA cache_size=-4096;
PRAGMA temp_store=FILE;
CREATE TABLE IF NOT EXISTS live_projection(id TEXT PRIMARY KEY, body BLOB NOT NULL);
CREATE TABLE IF NOT EXISTS live_records(
 key TEXT PRIMARY KEY,plan TEXT NOT NULL,unit TEXT NOT NULL,revision INTEGER NOT NULL,
 seq INTEGER NOT NULL,window_ns INTEGER NOT NULL,start_ns INTEGER NOT NULL,
 scored INTEGER NOT NULL,expired INTEGER NOT NULL,body BLOB NOT NULL,scored_seq INTEGER NOT NULL,
 UNIQUE(plan,unit,revision));
CREATE INDEX IF NOT EXISTS live_pending ON live_records(scored,expired,seq);
CREATE INDEX IF NOT EXISTS live_plan_pending ON live_records(plan,scored,expired);
CREATE INDEX IF NOT EXISTS live_window ON live_records(plan,start_ns,expired,seq);
CREATE INDEX IF NOT EXISTS live_event_range ON live_records(plan,expired,window_ns);
CREATE UNIQUE INDEX IF NOT EXISTS live_sequence ON live_records(seq);
CREATE TABLE IF NOT EXISTS live_units(plan TEXT,unit TEXT,first_seq INTEGER,window_ns INTEGER,
 latest_key TEXT,revision INTEGER,expired INTEGER,PRIMARY KEY(plan,unit));
CREATE INDEX IF NOT EXISTS live_latest ON live_units(plan,latest_key);
CREATE TABLE IF NOT EXISTS live_decisions(key TEXT,evaluator TEXT,plan TEXT,start_ns INTEGER,
 identity TEXT,decision TEXT,PRIMARY KEY(key,evaluator));
CREATE INDEX IF NOT EXISTS live_budget ON live_decisions(plan,start_ns,identity,decision);
CREATE TABLE IF NOT EXISTS live_assemblies(key TEXT PRIMARY KEY,plan TEXT,first_ns INTEGER,
 emitted INTEGER,body BLOB,due_ns INTEGER,revision INTEGER);
CREATE INDEX IF NOT EXISTS live_assembly_pending ON live_assemblies(emitted,due_ns,key);
CREATE TABLE IF NOT EXISTS live_expired(id TEXT PRIMARY KEY,plan TEXT,start_ns INTEGER,end_ns INTEGER,body BLOB);
CREATE INDEX IF NOT EXISTS live_expired_time ON live_expired(plan,start_ns,end_ns);
CREATE TABLE IF NOT EXISTS live_audits(id TEXT PRIMARY KEY,body BLOB);
CREATE TABLE IF NOT EXISTS live_emissions(assembly TEXT,revision INTEGER,key TEXT,PRIMARY KEY(assembly,revision));
CREATE TABLE IF NOT EXISTS live_controls(plan TEXT PRIMARY KEY,paused INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS live_snapshots(id TEXT,seq INTEGER,body BLOB,start_ns INTEGER,window_ns INTEGER,scored INTEGER,scored_seq INTEGER,latest INTEGER,PRIMARY KEY(id,seq));
CREATE INDEX IF NOT EXISTS live_snapshot_window ON live_snapshots(id,start_ns,latest);
`

func (m *LiveManager) project(db liveSQL, ev liveJournal, encoded []byte) error {
	if _, err := db.Exec("INSERT INTO live_projection(id,body) VALUES(?,?)", fmt.Sprintf("%09d", ev.Sequence), encoded); err != nil {
		return err
	}
	if r := ev.Record; r != nil {
		copy := *r
		if ev.Type == "event.accepted" {
			copy.Sequence = ev.Sequence
		}
		if copy.Unit == "" {
			return errors.New("online journal requires indexed unit identity; regenerate old development state")
		}
		key := liveKey(copy.PlanID, copy.Event.Source, copy.Event.EventID)
		body, err := json.Marshal(copy)
		if err != nil {
			return err
		}
		_, err = db.Exec(`INSERT INTO live_records VALUES(?,?,?,?,?,?,?,?,?,?,?)
 ON CONFLICT(key) DO UPDATE SET scored=excluded.scored,expired=excluded.expired,body=excluded.body,scored_seq=excluded.scored_seq`, key, copy.PlanID, copy.Unit, copy.Event.EvidenceRevision, copy.Sequence, copy.WindowTime.UnixNano(), copy.WindowStart.UnixNano(), copy.Case != nil, copy.Expired, body, copy.ScoredSequence)
		if err != nil {
			return err
		}
		_, err = db.Exec(`INSERT INTO live_units VALUES(?,?,?,?,?,?,?) ON CONFLICT(plan,unit) DO UPDATE SET
 latest_key=CASE WHEN excluded.revision>revision THEN excluded.latest_key ELSE latest_key END,
 revision=max(revision,excluded.revision),expired=max(expired,excluded.expired)`, copy.PlanID, copy.Unit, copy.Sequence, copy.WindowTime.UnixNano(), key, copy.Event.EvidenceRevision, copy.Expired)
		if err != nil {
			return err
		}
		for ref, decision := range copy.Decisions {
			identity := copy.DecisionIdentities[ref]
			_, err = db.Exec(`INSERT INTO live_decisions VALUES(?,?,?,?,?,?) ON CONFLICT(key,evaluator) DO UPDATE SET decision=excluded.decision`, key, ref, copy.PlanID, copy.WindowStart.UnixNano(), identity, decision)
			if err != nil {
				return err
			}
		}
		if key, ok := copy.Event.Case.Metadata["seven_eval_otlp_assembly"].(string); ok {
			if _, err = db.Exec("INSERT OR IGNORE INTO live_emissions VALUES(?,?,?)", key, copy.Event.EvidenceRevision, copy.Event.EventID); err != nil {
				return err
			}
			if _, err = db.Exec("UPDATE live_assemblies SET emitted=1 WHERE key=? AND revision=?", key, copy.Event.EvidenceRevision); err != nil {
				return err
			}
		}
	}
	if a := ev.Assembly; a != nil {
		body, err := json.Marshal(a)
		if err != nil {
			return err
		}
		_, err = db.Exec(`INSERT INTO live_assemblies VALUES(?,?,?,?,?,?,?) ON CONFLICT(key) DO UPDATE SET body=excluded.body,emitted=excluded.emitted,due_ns=excluded.due_ns,revision=excluded.revision`, a.Key, a.PlanID, a.FirstAt.UnixNano(), a.ExpiredSpans != nil, body, a.DueAt.UnixNano(), a.Revision)
		if err != nil {
			return err
		}
		if a.ExpiredSpans == nil {
			if _, err = db.Exec("UPDATE live_assemblies SET emitted=1 WHERE key=? AND EXISTS(SELECT 1 FROM live_emissions WHERE assembly=? AND revision=?)", a.Key, a.Key, a.Revision); err != nil {
				return err
			}
		}
	}
	if ev.Type == "plan.paused" || ev.Type == "plan.resumed" {
		if _, err := db.Exec("INSERT OR REPLACE INTO live_controls VALUES(?,?)", ev.PlanID, ev.Type == "plan.paused"); err != nil {
			return err
		}
	}
	if ev.Type == "window.expired" && ev.ExpiredScope != nil {
		b, _ := json.Marshal(ev.ExpiredScope)
		s := ev.ExpiredScope
		if _, err := db.Exec("INSERT OR REPLACE INTO live_expired VALUES(?,?,?,?,?)", ev.WindowID, s.PlanID, s.Start.UnixNano(), s.End.UnixNano(), b); err != nil {
			return err
		}
	}
	if ev.Retention != nil {
		b, _ := json.Marshal(ev.Retention)
		if _, err := db.Exec("INSERT OR REPLACE INTO live_audits VALUES(?,?)", ev.Retention.Digest, b); err != nil {
			return err
		}
	}
	return nil
}

func (m *LiveManager) ensureProjectionLocked() error {
	if !m.projectionDirty {
		return nil
	}
	if err := m.rebuild(); err != nil {
		log.Printf("online projection rebuild failed: %v", err)
		return fmt.Errorf("online projection recovery: %w", err)
	}
	m.projectionDirty = false
	return nil
}

func (m *LiveManager) recordLocked(key string) (*liveRecord, error) {
	if err := m.ensureProjectionLocked(); err != nil {
		return nil, err
	}
	var body []byte
	err := m.db.QueryRow("SELECT body FROM live_records WHERE key=?", key).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var r liveRecord
	err = decodeJSON(body, &r)
	return &r, err
}

// Bounded fetches close the SQLite cursor before calling workers or receivers.
func (m *LiveManager) recordBatchLocked(query string, args ...any) ([]liveRecord, error) {
	if err := m.ensureProjectionLocked(); err != nil {
		return nil, err
	}
	rows, err := m.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	batch := []liveRecord{}
	size := 0
	for rows.Next() {
		var b []byte
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		if len(batch) > 0 && size+len(b) > MaxMetadataBytes {
			break
		}
		var r liveRecord
		if err = decodeJSON(b, &r); err != nil {
			return nil, err
		}
		batch = append(batch, r)
		size += len(b)
	}
	return batch, rows.Err()
}

func (m *LiveManager) assemblyLocked(key string) (otlpAssembly, bool, error) {
	if err := m.ensureProjectionLocked(); err != nil {
		return otlpAssembly{}, false, err
	}
	var b []byte
	err := m.db.QueryRow("SELECT body FROM live_assemblies WHERE key=?", key).Scan(&b)
	if errors.Is(err, sql.ErrNoRows) {
		return otlpAssembly{}, false, nil
	}
	if err != nil {
		return otlpAssembly{}, false, err
	}
	var a otlpAssembly
	err = decodeJSON(b, &a)
	return a, true, err
}

func (m *LiveManager) pendingLocked(plan string) (int, error) {
	if err := m.ensureProjectionLocked(); err != nil {
		return 0, err
	}
	var n int
	err := m.db.QueryRow("SELECT count(*) FROM live_records WHERE plan=? AND scored=0 AND expired=0", plan).Scan(&n)
	return n, err
}

func (m *LiveManager) expiredWindowLocked(id string) (bool, error) {
	var n int
	if err := m.ensureProjectionLocked(); err != nil {
		return false, err
	}
	err := m.db.QueryRow("SELECT count(*) FROM live_expired WHERE id=?", id).Scan(&n)
	return n > 0, err
}

func (m *LiveManager) walkRecords(query string, args []any, visit func(string, *liveRecord) error) error {
	var after int
	for {
		m.mu.Lock()
		params := append(append([]any{}, args...), after, LiveReadBatch)
		batch, err := m.recordBatchLocked(query, params...)
		m.mu.Unlock()
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			return nil
		}
		for i := range batch {
			r := &batch[i]
			after = r.Sequence
			if err = visit(liveKey(r.PlanID, r.Event.Source, r.Event.EventID), r); err != nil {
				return err
			}
		}
	}
}

func (m *LiveManager) walkSnapshot(id string, visit func(liveRecord) error) error {
	var after int
	for {
		m.mu.Lock()
		batch, err := m.recordBatchLocked("SELECT body FROM live_snapshots WHERE id=? AND seq>? ORDER BY seq LIMIT ?", id, after, LiveReadBatch)
		m.mu.Unlock()
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			return nil
		}
		for _, r := range batch {
			after = r.Sequence
			if err = visit(r); err != nil {
				return err
			}
		}
	}
}

func (m *LiveManager) snapshotRangeLocked(id, plan string, from, to time.Time, latest bool) error {
	if err := m.ensureProjectionLocked(); err != nil {
		return err
	}
	filter := ""
	if latest {
		filter = " AND u.latest_key=r.key"
	}
	_, err := m.db.Exec(`INSERT OR IGNORE INTO live_snapshots SELECT ?,r.seq,r.body,r.start_ns,r.window_ns,r.scored,r.scored_seq,u.latest_key=r.key FROM live_records r JOIN live_units u ON u.plan=r.plan AND u.unit=r.unit WHERE r.plan=? AND r.expired=0 AND r.window_ns>=? AND r.window_ns<?`+filter, id, plan, from.UnixNano(), to.UnixNano())
	return err
}

func (m *LiveManager) walkRecordsLocked(visit func(string, *liveRecord) error) error {
	var after int
	for {
		batch, err := m.recordBatchLocked("SELECT body FROM live_records WHERE seq>? ORDER BY seq LIMIT ?", after, LiveReadBatch)
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			return nil
		}
		for i := range batch {
			r := &batch[i]
			after = r.Sequence
			if err = visit(liveKey(r.PlanID, r.Event.Source, r.Event.EventID), r); err != nil {
				return err
			}
		}
	}
}

func (m *LiveManager) walkAssembliesLocked(visit func(string, otlpAssembly) error) error {
	after := ""
	for {
		rows, err := m.db.Query("SELECT key FROM live_assemblies WHERE key>? ORDER BY key LIMIT ?", after, LiveReadBatch)
		if err != nil {
			return err
		}
		keys := []string{}
		for rows.Next() {
			var k string
			if err = rows.Scan(&k); err != nil {
				rows.Close()
				return err
			}
			keys = append(keys, k)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if len(keys) == 0 {
			return nil
		}
		for _, k := range keys {
			a, _, err := m.assemblyLocked(k)
			if err != nil {
				return err
			}
			after = k
			if err = visit(k, a); err != nil {
				return err
			}
		}
	}
}

// Maintenance retains only bounded identity metadata, never historical evidence.
func (m *LiveManager) retentionViewLocked() (map[string]*liveRecord, map[string]otlpAssembly, map[string]ExpiredWindow, []RetentionAudit, error) {
	bytes := 0
	charge := func(v any) error {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		bytes += len(b)
		if bytes > 64<<20 {
			return errors.New("retention metadata capacity exceeded")
		}
		return nil
	}
	records := map[string]*liveRecord{}
	assemblies := map[string]otlpAssembly{}
	scopes := map[string]ExpiredWindow{}
	audits := []RetentionAudit{}
	err := m.walkRecordsLocked(func(key string, r *liveRecord) error {
		if len(records) >= MaxRetentionIdentities {
			return errors.New("retention identity limit exceeded")
		}
		copy := *r
		copy.Event.Case = Case{}
		copy.Event.Result = SubjectResult{}
		if copy.Case != nil {
			copy.Case = &CaseResult{}
		}
		copy.Decisions = nil
		copy.DecisionIdentities = nil
		if err := charge(copy); err != nil {
			return err
		}
		records[key] = &copy
		return nil
	})
	if err != nil {
		return nil, nil, nil, nil, err
	}
	err = m.walkAssembliesLocked(func(key string, a otlpAssembly) error {
		if len(assemblies) >= MaxRetentionIdentities {
			return errors.New("retention identity limit exceeded")
		}
		a.ContentDigest = Digest(a.Spans)
		source := attrs(a.Resource)["service.name"]
		a.Resource = []otlpAttr{{Key: "service.name"}}
		a.Resource[0].Value.StringValue = source
		a.Spans = nil
		if a.ExpiredSpans != nil {
			a.ExpiredSpans = map[string]string{}
		}
		assemblies[key] = a
		return charge(a)
	})
	if err != nil {
		return nil, nil, nil, nil, err
	}
	rows, err := m.db.Query("SELECT id,body FROM live_expired")
	if err != nil {
		return nil, nil, nil, nil, err
	}
	for rows.Next() {
		if len(scopes) >= MaxRetentionIdentities {
			rows.Close()
			return nil, nil, nil, nil, errors.New("retention identity limit exceeded")
		}
		var id string
		var b []byte
		var s ExpiredWindow
		if err = rows.Scan(&id, &b); err == nil {
			err = decodeJSON(b, &s)
		}
		if err != nil {
			rows.Close()
			return nil, nil, nil, nil, err
		}
		scopes[id] = s
		if err = charge(s); err != nil {
			rows.Close()
			return nil, nil, nil, nil, err
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	rows, err = m.db.Query("SELECT body FROM live_audits")
	if err != nil {
		return nil, nil, nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		if len(audits) >= MaxRetentionIdentities {
			return nil, nil, nil, nil, errors.New("retention audit limit exceeded")
		}
		var b []byte
		var a RetentionAudit
		if err = rows.Scan(&b); err == nil {
			err = decodeJSON(b, &a)
		}
		if err != nil {
			return nil, nil, nil, nil, err
		}
		audits = append(audits, a)
		if err = charge(a); err != nil {
			return nil, nil, nil, nil, err
		}
	}
	return records, assemblies, scopes, audits, rows.Err()
}

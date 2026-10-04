package core

import (
	"encoding/json"
	"time"
)

const Version = "1.0"
const MaxCases = 1000
const MaxFileBytes = 32 << 20
const MaxLiveEventBytes = 1 << 20
const MaxLiveBatch = 100
const MaxLivePending = 10000

type Package struct {
	SchemaVersion string `json:"schema_version" yaml:"schema_version"`
	ID            string `json:"id" yaml:"id"`
	Version       string `json:"version" yaml:"version"`
	Name          string `json:"name" yaml:"name"`
}
type Asset struct {
	Kind       string         `json:"kind"`
	ID         string         `json:"id"`
	Version    string         `json:"version"`
	Source     string         `json:"source"`
	Path       string         `json:"path"`
	Status     string         `json:"status"`
	Message    string         `json:"message,omitempty"`
	Descriptor map[string]any `json:"descriptor,omitempty"`
}
type Discovery struct {
	Package Package `json:"package"`
	Assets  []Asset `json:"assets"`
}
type Plan struct {
	TargetParameters map[string]any `json:"target_parameters,omitempty" yaml:"target_parameters,omitempty"`
	Candidates       []Candidate    `json:"candidates,omitempty" yaml:"candidates,omitempty"`
	SchemaVersion    string         `json:"schema_version" yaml:"schema_version"`
	ID               string         `json:"id" yaml:"id"`
	Preset           string         `json:"preset" yaml:"preset"`
	Mode             string         `json:"mode" yaml:"mode"`
	Dataset          string         `json:"dataset,omitempty" yaml:"dataset"`
	Source           string         `json:"source,omitempty" yaml:"source,omitempty"`
	Mapping          string         `json:"mapping,omitempty" yaml:"mapping,omitempty"`
	Parameters       map[string]any `json:"parameters,omitempty" yaml:"parameters,omitempty"`
	Subject          string         `json:"subject,omitempty" yaml:"subject,omitempty"`
	Evaluators       *[]string      `json:"evaluators" yaml:"evaluators"`
	Processors       *[]string      `json:"processors" yaml:"processors"`
	Conclusions      *[]string      `json:"conclusions" yaml:"conclusions"`
	TimeoutSeconds   int            `json:"timeout_seconds" yaml:"timeout_seconds"`
	Stream           *StreamConfig  `json:"stream,omitempty" yaml:"stream,omitempty"`
}

// Candidate changes target behavior, never the shared evaluator configuration.
type Candidate struct {
	ID               string         `json:"id" yaml:"id"`
	TargetParameters map[string]any `json:"target_parameters" yaml:"target_parameters"`
}

// Comparison freezes the common normalized case identities before any invocation.
type Comparison struct {
	BaselineID  string   `json:"baseline_id"`
	CaseIDs     []string `json:"case_ids"`
	InputDigest string   `json:"input_digest"`
}

// ParameterReceipt is a target assertion, not independent proof of implementation.
type ParameterReceipt struct {
	Requested map[string]any `json:"requested"`
	Applied   map[string]any `json:"applied,omitempty"`
	State     string         `json:"state"`
}

// StreamConfig is intentionally small: the plan owns scoring policy while the
// receiver owns transport. It keeps online streams on the same recipe seams as
// offline plans without giving a production event a Subject to invoke.
type StreamConfig struct {
	Unit                   string   `json:"unit" yaml:"unit"`
	WindowSeconds          int      `json:"window_seconds" yaml:"window_seconds"`
	RefreshSeconds         int      `json:"refresh_seconds" yaml:"refresh_seconds"`
	AllowedLatenessSeconds int      `json:"allowed_lateness_seconds" yaml:"allowed_lateness_seconds"`
	QuietSeconds           int      `json:"quiet_seconds" yaml:"quiet_seconds"`
	EvidenceTimeoutSeconds int      `json:"evidence_timeout_seconds" yaml:"evidence_timeout_seconds"`
	MaxPending             int      `json:"max_pending" yaml:"max_pending"`
	JudgeSampleRate        *float64 `json:"judge_sample_rate,omitempty" yaml:"judge_sample_rate,omitempty"`
	JudgePerWindowBudget   *int     `json:"judge_per_window_budget,omitempty" yaml:"judge_per_window_budget,omitempty"`
}
type Effective struct {
	NodeRuntime *NodeRuntime      `json:"node_runtime,omitempty"`
	Files       map[string]string `json:"files"`
	Plan        Plan              `json:"plan"`
	Evaluators  []string          `json:"evaluators"`
	Processors  []string          `json:"processors"`
	Conclusions []string          `json:"conclusions"`
	Assets      []Asset           `json:"assets"`
	Digest      string            `json:"digest"`
}
type NodeRuntime struct {
	SnapshotID string `json:"snapshot_id"`
	Version    string `json:"version"`
}
type Case struct {
	ID         string         `json:"id"`
	Inputs     any            `json:"inputs"`
	References map[string]any `json:"references,omitempty"`
	Response   any            `json:"response,omitempty"`
	Contexts   []any          `json:"contexts,omitempty"`
	Result     *SubjectResult `json:"result,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	Episode    *Episode       `json:"episode,omitempty"`
}
type Turn struct {
	ID   string `json:"id"`
	Case Case   `json:"case"`
}
type Episode struct {
	ID    string `json:"id"`
	Turns []Turn `json:"turns"`
}
type EpisodeResult struct {
	ID        string   `json:"id"`
	Status    string   `json:"status"`
	Session   any      `json:"session,omitempty"`
	CaseIDs   []string `json:"case_ids"`
	Completed int      `json:"completed"`
}
type Evidence struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Value any    `json:"value"`
}
type SubjectResult struct {
	ParameterReceipt *ParameterReceipt   `json:"parameter_receipt,omitempty"`
	Response         any                 `json:"response"`
	Contexts         []any               `json:"contexts,omitempty"`
	Usage            map[string]any      `json:"usage,omitempty"`
	LatencyMS        *float64            `json:"latency_ms,omitempty"`
	Retrieval        []RetrievalDocument `json:"retrieval,omitempty"`
	Tools            []ToolEvidence      `json:"tools,omitempty"`
	Citations        []Citation          `json:"citations,omitempty"`
	Trace            []any               `json:"trace,omitempty"`
}

// MarshalJSON preserves provided empty evidence separately from absent evidence.
func (r SubjectResult) MarshalJSON() ([]byte, error) {
	type plain SubjectResult
	b, err := json.Marshal(plain(r))
	if err != nil {
		return nil, err
	}
	var fields map[string]any
	if err = decodeJSON(b, &fields); err != nil {
		return nil, err
	}
	if r.Retrieval != nil {
		fields["retrieval"] = r.Retrieval
	}
	if r.Tools != nil {
		fields["tools"] = r.Tools
	}
	if r.Citations != nil {
		fields["citations"] = r.Citations
	}
	return json.Marshal(fields)
}

type Metric struct {
	ID           string         `json:"id"`
	MetricID     string         `json:"metric_id"`
	CaseID       string         `json:"case_id"`
	Status       string         `json:"status"`
	Value        any            `json:"value"`
	Reason       string         `json:"reason,omitempty"`
	Evaluator    string         `json:"evaluator"`
	Parameters   map[string]any `json:"parameters,omitempty"`
	EvidenceRefs []string       `json:"evidence_refs,omitempty"`
}
type CaseResult struct {
	RedactionApplied bool          `json:"redaction_applied"`
	Case             Case          `json:"case"`
	Status           string        `json:"status"`
	Reason           string        `json:"reason,omitempty"`
	Attempt          int           `json:"attempt"`
	Result           SubjectResult `json:"result"`
	Evidence         []Evidence    `json:"evidence"`
	Metrics          []Metric      `json:"metrics"`
}
type ProcessorOutput struct {
	Report      *ReportMapping `json:"report,omitempty"`
	Processor   string         `json:"processor"`
	Data        map[string]any `json:"data"`
	InputDigest string         `json:"input_digest"`
}
type ReportMapping struct {
	Version string            `json:"version"`
	KPIs    map[string]string `json:"kpis"`
}
type Conclusion struct {
	Handler string `json:"handler"`
	Code    string `json:"code"`
	Level   string `json:"level"`
	Summary string `json:"summary"`
}
type Run struct {
	Comparison      *Comparison       `json:"comparison,omitempty"`
	SchemaVersion   string            `json:"schema_version"`
	ID              string            `json:"id"`
	Package         Package           `json:"package"`
	CreatedAt       time.Time         `json:"created_at"`
	Status          string            `json:"status"`
	Message         string            `json:"message,omitempty"`
	Total           int               `json:"total"`
	Completed       int               `json:"completed"`
	Effective       Effective         `json:"effective"`
	DatasetDigest   string            `json:"dataset_digest"`
	Cases           []CaseResult      `json:"cases"`
	Outputs         []ProcessorOutput `json:"outputs"`
	Conclusions     []Conclusion      `json:"conclusions"`
	Catalogs        map[string]any    `json:"catalogs"`
	Scope           *WindowScope      `json:"scope,omitempty"`
	DatasetRevision *DatasetRevision  `json:"dataset_revision,omitempty"`
	Episodes        []EpisodeResult   `json:"episodes,omitempty"`
}

// DatasetRevision freezes input provenance independently of a transport.
type DatasetRevision struct {
	SourceID       string            `json:"source_id"`
	SourceVersion  string            `json:"source_version"`
	MappingID      string            `json:"mapping_id,omitempty"`
	MappingVersion string            `json:"mapping_version,omitempty"`
	Files          map[string]string `json:"files"`
	Digest         string            `json:"digest"`
}
type RetrievalDocument struct {
	ID    string   `json:"id"`
	Text  string   `json:"text"`
	Score *float64 `json:"score,omitempty"`
}
type ToolEvidence struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
	Status    string         `json:"status"`
	Result    any            `json:"result,omitempty"`
}
type Citation struct {
	DocumentID string `json:"document_id"`
	Start      *int   `json:"start,omitempty"`
	End        *int   `json:"end,omitempty"`
}
type WindowScope struct {
	StatisticsAvailable bool           `json:"statistics_available"`
	Facts               []WindowFact   `json:"facts,omitempty"`
	BacklogFacts        []WindowFact   `json:"backlog_facts,omitempty"`
	Coverage            *float64       `json:"coverage,omitempty"`
	Reasons             map[string]int `json:"reasons,omitempty"`
	Anomalies           int            `json:"anomalies"`
	Skipped             int            `json:"skipped"`
	BudgetExhausted     int            `json:"budget_exhausted"`
	Unscored            int            `json:"unscored"`
	Kind                string         `json:"kind"`
	PlanID              string         `json:"plan_id"`
	Source              string         `json:"source,omitempty"`
	Start               time.Time      `json:"start"`
	End                 time.Time      `json:"end"`
	Watermark           time.Time      `json:"watermark"`
	Revision            int            `json:"revision"`
	State               string         `json:"state"`
	Received            int            `json:"received"`
	Eligible            int            `json:"eligible"`
	Sampled             int            `json:"sampled"`
	Scored              int            `json:"scored"`
	NotComputable       int            `json:"not_computable"`
	Failed              int            `json:"failed"`
	Pending             int            `json:"pending"`
	P50LatencyMS        *float64       `json:"p50_latency_ms,omitempty"`
	P95LatencyMS        *float64       `json:"p95_latency_ms,omitempty"`
	JudgeSampleRate     *float64       `json:"judge_sample_rate,omitempty"`
	JudgeBudget         *int           `json:"judge_per_window_budget,omitempty"`
}
type WindowFact struct {
	Origin     map[string]string `json:"origin"`
	CaseID     string            `json:"case_id"`
	ReceivedAt time.Time         `json:"received_at"`
	StartedAt  *time.Time        `json:"started_at,omitempty"`
	ScoredAt   *time.Time        `json:"scored_at,omitempty"`
	Revision   int               `json:"evidence_revision"`
}
type ProductionEvent struct {
	SchemaVersion    string        `json:"schema_version"`
	Source           string        `json:"source"`
	Project          string        `json:"project"`
	EventID          string        `json:"event_id"`
	EventTime        time.Time     `json:"event_time"`
	TraceID          string        `json:"trace_id"`
	TurnID           string        `json:"turn_id,omitempty"`
	EvidenceRevision int           `json:"evidence_revision"`
	Case             Case          `json:"case"`
	Result           SubjectResult `json:"result"`
}
type IngestBatch struct {
	PlanID string            `json:"plan_id"`
	Events []ProductionEvent `json:"events"`
}
type IngestItem struct {
	EventID string `json:"event_id"`
	Status  string `json:"status"`
	Code    string `json:"code,omitempty"`
}
type IngestResult struct {
	Accepted  int          `json:"accepted"`
	Duplicate int          `json:"duplicate"`
	Rejected  int          `json:"rejected"`
	Items     []IngestItem `json:"items"`
}
type Event struct {
	Sequence int             `json:"sequence"`
	Type     string          `json:"type"`
	Time     time.Time       `json:"time"`
	Run      json.RawMessage `json:"run,omitempty"`
	Data     any             `json:"data,omitempty"`
}
type HTTPConfig struct {
	AppliedParametersPath string         `json:"applied_parameters_path" yaml:"applied_parameters_path"`
	ID                    string         `json:"id" yaml:"id"`
	Version               string         `json:"version" yaml:"version"`
	Type                  string         `json:"type" yaml:"type"`
	Endpoint              string         `json:"endpoint" yaml:"endpoint"`
	Request               map[string]any `json:"request" yaml:"request"`
	ResponsePath          string         `json:"response_path" yaml:"response_path"`
	ContextsPath          string         `json:"contexts_path" yaml:"contexts_path"`
	UsagePath             string         `json:"usage_path" yaml:"usage_path"`
	AuthEnv               string         `json:"auth_env" yaml:"auth_env"`
	RetrievalPath         string         `json:"retrieval_path" yaml:"retrieval_path"`
	ToolsPath             string         `json:"tools_path" yaml:"tools_path"`
	CitationsPath         string         `json:"citations_path" yaml:"citations_path"`
	WorkspaceID           string         `json:"workspace_id" yaml:"workspace_id"`
}

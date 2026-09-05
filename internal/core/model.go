package core

import (
	"encoding/json"
	"time"
)

const Version = "1.0"
const MaxCases = 1000
const MaxFileBytes = 32 << 20

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
	SchemaVersion  string    `json:"schema_version" yaml:"schema_version"`
	ID             string    `json:"id" yaml:"id"`
	Preset         string    `json:"preset" yaml:"preset"`
	Mode           string    `json:"mode" yaml:"mode"`
	Dataset        string    `json:"dataset" yaml:"dataset"`
	Subject        string    `json:"subject,omitempty" yaml:"subject,omitempty"`
	Evaluators     *[]string `json:"evaluators" yaml:"evaluators"`
	Processors     *[]string `json:"processors" yaml:"processors"`
	Conclusions    *[]string `json:"conclusions" yaml:"conclusions"`
	TimeoutSeconds int       `json:"timeout_seconds" yaml:"timeout_seconds"`
}
type Effective struct {
	Files       map[string]string `json:"files"`
	Plan        Plan              `json:"plan"`
	Evaluators  []string          `json:"evaluators"`
	Processors  []string          `json:"processors"`
	Conclusions []string          `json:"conclusions"`
	Assets      []Asset           `json:"assets"`
	Digest      string            `json:"digest"`
}
type Case struct {
	ID         string         `json:"id"`
	Inputs     any            `json:"inputs"`
	References map[string]any `json:"references,omitempty"`
	Response   any            `json:"response,omitempty"`
	Contexts   []any          `json:"contexts,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}
type Evidence struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Value any    `json:"value"`
}
type SubjectResult struct {
	Response  any            `json:"response"`
	Contexts  []any          `json:"contexts,omitempty"`
	Usage     map[string]any `json:"usage,omitempty"`
	LatencyMS *float64       `json:"latency_ms,omitempty"`
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
	Processor   string         `json:"processor"`
	Data        map[string]any `json:"data"`
	InputDigest string         `json:"input_digest"`
}
type Conclusion struct {
	Handler string `json:"handler"`
	Code    string `json:"code"`
	Level   string `json:"level"`
	Summary string `json:"summary"`
}
type Run struct {
	SchemaVersion string            `json:"schema_version"`
	ID            string            `json:"id"`
	Package       Package           `json:"package"`
	CreatedAt     time.Time         `json:"created_at"`
	Status        string            `json:"status"`
	Message       string            `json:"message,omitempty"`
	Total         int               `json:"total"`
	Completed     int               `json:"completed"`
	Effective     Effective         `json:"effective"`
	DatasetDigest string            `json:"dataset_digest"`
	Cases         []CaseResult      `json:"cases"`
	Outputs       []ProcessorOutput `json:"outputs"`
	Conclusions   []Conclusion      `json:"conclusions"`
	Catalogs      map[string]any    `json:"catalogs"`
}
type Event struct {
	Sequence int             `json:"sequence"`
	Type     string          `json:"type"`
	Time     time.Time       `json:"time"`
	Run      json.RawMessage `json:"run,omitempty"`
	Data     any             `json:"data,omitempty"`
}
type HTTPConfig struct {
	ID           string         `json:"id" yaml:"id"`
	Version      string         `json:"version" yaml:"version"`
	Type         string         `json:"type" yaml:"type"`
	Endpoint     string         `json:"endpoint" yaml:"endpoint"`
	Request      map[string]any `json:"request" yaml:"request"`
	ResponsePath string         `json:"response_path" yaml:"response_path"`
	ContextsPath string         `json:"contexts_path" yaml:"contexts_path"`
	UsagePath    string         `json:"usage_path" yaml:"usage_path"`
	AuthEnv      string         `json:"auth_env" yaml:"auth_env"`
}

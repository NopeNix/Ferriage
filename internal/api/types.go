// Package api is the typed peer API contract (CONCEPT: API-first).
//
// One definition per JSON shape the peer serves, shared by the HTTP
// handlers (they marshal these structs, never ad-hoc maps) and the
// CLI (which unmarshals them). The OpenAPI document served at
// /openapi.json is generated from the same hand-written shapes, so a
// field rename breaks the compiler here instead of silently drifting
// the web client (web/src/api/types.ts is generated from the
// OpenAPI document).
package api

// Service is one Compose service within a stack.
type Service struct {
	Name      string `json:"name"`
	Container string `json:"container"`
	Image     string `json:"image"`
	State     string `json:"state"`
	Running   bool   `json:"running"`
}

// Stack is one adopted Compose project.
type Stack struct {
	Name           string    `json:"name"`
	ComposeFile    string    `json:"compose_file,omitempty"`
	WorkingDir     string    `json:"working_dir,omitempty"`
	Services       []Service `json:"services"`
	RunningCount   int       `json:"running_count"`
	ContainerCount int       `json:"container_count"`
}

// StacksResponse is the body of GET /stacks.
type StacksResponse struct {
	Stacks []Stack `json:"stacks"`
}

// PeerResponse is the body of GET /peer.
type PeerResponse struct {
	PeerID  string `json:"peer_id"`
	Version string `json:"version"`
	Commit  string `json:"commit"`
}

// DeploySvc is one service's gated state after a deploy.
type DeploySvc struct {
	Name      string `json:"name"`
	Health    string `json:"health"`
	HasHealth bool   `json:"has_healthcheck"`
	Running   bool   `json:"running"`
}

// DeployResponse is the body of POST /stacks/{name}/deploy.
type DeployResponse struct {
	Stack    string      `json:"stack"`
	Services []DeploySvc `json:"services"`
	Healthy  bool        `json:"healthy"`
	Detail   string      `json:"detail,omitempty"`
}

// VolumeDesc describes one captured volume in a snapshot.
type VolumeDesc struct {
	Name string `json:"name"`
}

// SnapshotManifest is the manifest.json embedded in (and returned
// by) snapshot export/import (ADR 0002).
type SnapshotManifest struct {
	FormatVersion int          `json:"format_version"`
	Stack         string       `json:"stack"`
	CreatedAt     string       `json:"created_at"`
	CreatedBy     string       `json:"created_by,omitempty"`
	Files         []string     `json:"files"`
	Volumes       []VolumeDesc `json:"volumes,omitempty"`
}

// ErrorResponse is the uniform error body.
type ErrorResponse struct {
	Error string `json:"error"`
}

// LogLine is one line of a container's log output.
type LogLine struct {
	Ts      string `json:"ts,omitempty"`
	Stream  string `json:"stream"`
	Message string `json:"message"`
}

// LogsResponse is the body of GET /stacks/{name}/services/{svc}/logs.
type LogsResponse struct {
	Container string    `json:"container"`
	Lines     []LogLine `json:"lines"`
	Truncated bool      `json:"truncated"`
}

// ContainerStats is one container's live resource snapshot.
type ContainerStats struct {
	Container     string  `json:"container"`
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryBytes   uint64  `json:"memory_bytes"`
	MemoryLimit   uint64  `json:"memory_limit"`
	MemoryPercent float64 `json:"memory_percent"`
	NetRxBytes    uint64  `json:"net_rx_bytes"`
	NetTxBytes    uint64  `json:"net_tx_bytes"`
}

// StatsResponse is the body of GET /stacks/{name}/stats.
type StatsResponse struct {
	Stats []ContainerStats `json:"stats"`
}

// ExecRequest is the body of POST /stacks/{name}/services/{svc}/exec.
type ExecRequest struct {
	Cmd []string `json:"cmd"`
	Tty bool     `json:"tty,omitempty"`
}

// ExecResponse is the body of POST /stacks/{name}/services/{svc}/exec.
type ExecResponse struct {
	ExecID    string `json:"exec_id"`
	Running   bool   `json:"running"`
	ExitCode  int    `json:"exit_code"`
	Output    string `json:"output"`
	Truncated bool   `json:"truncated"`
}

// ComposeResponse is the body of GET /stacks/{name}/compose.
type ComposeResponse struct {
	Name            string `json:"name"`
	File            string `json:"file"`
	Content         string `json:"content"`
	ComposeFileUsed string `json:"compose_file_used,omitempty"`
}

// ComposeSaveRequest is the body of PUT /stacks/{name}/compose.
type ComposeSaveRequest struct {
	Content string `json:"content"`
}

// OnboardRequest is the body of POST /hosts/onboard (ADR 0006).
// Key-only auth: the private key is passed in the body and kept
// in memory for one SSH connection only.
type OnboardRequest struct {
	Address    string `json:"address"`
	User       string `json:"user,omitempty"`
	PrivateKey string `json:"private_key"`
	Image      string `json:"image,omitempty"`
	PeerURL    string `json:"peer_url,omitempty"`
	Port       int    `json:"port,omitempty"`
}

// OnboardStep is one onboarding phase (ssh, docker-check, ...).
type OnboardStep struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

// OnboardResponse is the body of POST /hosts/onboard. On failure
// the peer still returns HTTP 502 with the partial result and a
// non-empty Error, so the UI can show how far onboarding got.
type OnboardResponse struct {
	Address   string        `json:"address"`
	Docker    string        `json:"docker"`
	Compose   string        `json:"compose"`
	Container string        `json:"container"`
	PeerURL   string        `json:"peer_url"`
	Healthy   bool          `json:"healthy"`
	Steps     []OnboardStep `json:"steps"`
	Error     string        `json:"error,omitempty"`
}

// NotifyStatus is the body of GET /notify (ADR 0007). Redacted
// by design: it reports whether notifications are configured and
// the outcome of the last send, never the URLs themselves (they
// are secrets and enter only via FERRIAGE_NOTIFY_URLS).
type NotifyStatus struct {
	Enabled   bool   `json:"enabled"`
	Channels  int    `json:"channels"`
	LastEvent string `json:"last_event,omitempty"`
	LastAt    string `json:"last_at,omitempty"`
	LastErr   string `json:"last_error,omitempty"`
}

// NotifyTestResponse is the body of POST /notify/test.
type NotifyTestResponse struct {
	Sent  bool   `json:"sent"`
	Event string `json:"event"`
}

// AuditEntry is one audit-log line (ADR 0008): who did what, when.
// Actor is the reported identity (BFF session sub via
// X-Ferriage-Actor, or "unknown" for unauthenticated peer calls).
// Status is the HTTP status of the recorded operation. Detail is
// short and secret-free (never keys, bodies, or URLs).
type AuditEntry struct {
	Ts     string `json:"ts"`
	Actor  string `json:"actor"`
	Action string `json:"action"`
	Target string `json:"target,omitempty"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// AuditResponse is the body of GET /audit (newest first).
type AuditResponse struct {
	Entries []AuditEntry `json:"entries"`
}

// UpdateService is one service's digest-check outcome (ADR 0009).
// Local/Remote are content digests (never empty on success).
// Outcome is one of up-to-date, available, unresolvable.
type UpdateService struct {
	Service string `json:"service"`
	Image   string `json:"image"`
	Local   string `json:"local,omitempty"`
	Remote  string `json:"remote,omitempty"`
	Outcome string `json:"outcome"`
	Detail  string `json:"detail,omitempty"`
}

// UpdateRun is one auto-update run's outcome.
type UpdateRun struct {
	Stack    string `json:"stack"`
	At       string `json:"at"`
	Snapshot string `json:"snapshot,omitempty"`
	OK       bool   `json:"ok"`
	Detail   string `json:"detail,omitempty"`
}

// UpdateStatus is one stack's update report (ADR 0009).
// State is one of up-to-date, available, unresolvable,
// not-running, check-failed. Policy is poll or auto.
// Stateful marks stacks whose rendered Compose declares named
// volumes; stateful stacks never inherit `auto` implicitly.
type UpdateStatus struct {
	Stack     string          `json:"stack"`
	State     string          `json:"state"`
	Policy    string          `json:"policy"`
	Stateful  bool            `json:"stateful"`
	CheckedAt string          `json:"checked_at,omitempty"`
	Services  []UpdateService `json:"services"`
	LastRun   *UpdateRun      `json:"last_run,omitempty"`
	Snapshot  string          `json:"snapshot,omitempty"`
	Error     string          `json:"error,omitempty"`
}

// UpdateList is the body of GET /update.
type UpdateList struct {
	Stacks []UpdateStatus `json:"stacks"`
}

// UpdatePolicyRequest is the body of PUT /update/{name}/policy.
// Either field may be set; setting interval to "" keeps the
// existing override. There is no way to clear a policy via empty
// — the field is omitted-or-set, never half-cleared.
type UpdatePolicyRequest struct {
	Policy   string `json:"policy,omitempty"`
	Interval string `json:"interval,omitempty"`
}

// UpdatePolicyResponse is the body of PUT /update/{name}/policy.
type UpdatePolicyResponse struct {
	Stack    string `json:"stack"`
	Policy   string `json:"policy"`
	Interval string `json:"interval"`
	Stateful bool   `json:"stateful"`
}

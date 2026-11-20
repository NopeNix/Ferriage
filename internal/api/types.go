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

// Package version exposes build-time identity for the peer binary.
package version

// Version and Commit are overridden at image build time via -ldflags
// (Phase 1.6). Defaults keep local `go test` meaningful.
var Version = "dev"

var Commit = "unknown"

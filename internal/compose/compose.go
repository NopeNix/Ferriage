// Package compose reads and writes a stack's Compose file in the
// state directory. State stays files (CONCEPT req. 6): the peer
// never duplicates compose content into a database; PUT replaces
// the file atomically so a crash cannot leave a half-written
// compose behind. The web editor (Monaco, Phase 3 item 2)
// validates against the compose JSON schema client-side; the peer
// only stores bytes (ADR 0005).
package compose

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/NopeNix/Ferriage/internal/api"
)

// MaxBytes caps a single compose file write (4MB).
const MaxBytes = 4 << 20

// Handler serves GET and PUT /stacks/{name}/compose.
type Handler struct {
	StateDir string
}

// Register wires the compose routes onto the peer mux.
func Register(mux *http.ServeMux, stateDir string) {
	h := &Handler{StateDir: stateDir}
	mux.HandleFunc("GET /stacks/{name}/compose", h.serveGet)
	mux.HandleFunc("PUT /stacks/{name}/compose", h.servePut)
}

func (h *Handler) file(name string) string {
	return filepath.Join(h.StateDir, name, "compose.yaml")
}

func (h *Handler) serveGet(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !validName(name) {
		writeErr(w, http.StatusBadRequest, "bad stack name")
		return
	}
	data, err := os.ReadFile(h.file(name))
	if err != nil {
		writeErr(w, http.StatusNotFound, "compose file not found")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(api.ComposeResponse{
		Name:            name,
		File:            "compose.yaml",
		Content:         string(data),
		ComposeFileUsed: "compose.yaml",
	})
}

func (h *Handler) servePut(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !validName(name) {
		writeErr(w, http.StatusBadRequest, "bad stack name")
		return
	}
	var req api.ComposeSaveRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxBytes+1))
	if err := dec.Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad compose save request")
		return
	}
	if len(req.Content) > MaxBytes {
		writeErr(w, http.StatusRequestEntityTooLarge, "compose file too large")
		return
	}
	path := h.file(name)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			writeErr(w, http.StatusNotFound, "compose file not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "compose file check failed")
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(req.Content), 0o644); err != nil {
		writeErr(w, http.StatusInternalServerError, "write compose file failed")
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		writeErr(w, http.StatusInternalServerError, "replace compose file failed")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(api.ComposeResponse{
		Name:            name,
		File:            "compose.yaml",
		Content:         req.Content,
		ComposeFileUsed: "compose.yaml",
	})
}

func validName(s string) bool {
	if s == "" || s == "." || s == ".." {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
		default:
			return false
		}
	}
	return true
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(api.ErrorResponse{Error: msg})
}

// Package snapshot serves the export/import endpoints of ADR 0002:
// POST /stacks/{name}/export streams a snapshot archive;
// POST /stacks/{name}/import restores one from the request body.
package snapshot

import (
	"encoding/json"
	"net/http"

	"github.com/NopeNix/Ferriage/internal/api"
)

// clientFor returns a Backend for the request's daemon; created
// per request like internal/stacks (no cached broken client).
func clientFor(r *http.Request) (Backend, func(), error) {
	return BackendFor()
}

// Register serves /stacks/{name}/export and /stacks/{name}/import,
// rooted at the state directory, on the peer's mux.
func Register(mux *http.ServeMux, stateDir, peerID string) {
	h := &Handler{StateDir: stateDir, PeerID: peerID}
	mux.HandleFunc("POST /stacks/{name}/export", h.export)
	mux.HandleFunc("POST /stacks/{name}/import", h.importSnapshot)
}

type Handler struct {
	StateDir string
	PeerID   string
}

func (h *Handler) export(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	name := r.PathValue("name")
	if name == "" || name == "." || name == ".." {
		writeErr(w, http.StatusBadRequest, "bad stack name")
		return
	}
	backend, done, err := clientFor(r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer done()
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`.tar.gz"`)
	if err := Export(r.Context(), w, h.StateDir, name, h.PeerID, backend); err != nil {
		writeErr(w, statusFor(err), err.Error())
		return
	}
}

func (h *Handler) importSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	name := r.PathValue("name")
	if name == "" || name == "." || name == ".." {
		writeErr(w, http.StatusBadRequest, "bad stack name")
		return
	}
	backend, done, err := clientFor(r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer done()
	// Snapshots are operator-sized; cap the body rather than
	// buffering unbounded input.
	r.Body = http.MaxBytesReader(w, r.Body, 1<<31)
	m, err := Restore(r.Context(), r.Body, h.StateDir, name, backend)
	if err != nil {
		writeErr(w, statusFor(err), err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toManifest(m))
}

func statusFor(err error) int {
	if e, ok := err.(*Error); ok {
		switch e.Code {
		case "bad_request":
			return http.StatusBadRequest
		case "not_found":
			return http.StatusNotFound
		}
	}
	return http.StatusInternalServerError
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(api.ErrorResponse{Error: msg})
}

// Compile-time check: dockerBackend satisfies Backend.
var _ Backend = (*dockerBackend)(nil)

// toManifest maps snapshot.Manifest onto the typed API contract.
func toManifest(m *Manifest) api.SnapshotManifest {
	vols := make([]api.VolumeDesc, 0, len(m.Volumes))
	for _, v := range m.Volumes {
		vols = append(vols, api.VolumeDesc{Name: v.Name})
	}
	return api.SnapshotManifest{
		FormatVersion: m.FormatVersion,
		Stack:         m.Stack,
		CreatedAt:     m.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		CreatedBy:     m.CreatedBy,
		Files:         m.Files,
		Volumes:       vols,
	}
}

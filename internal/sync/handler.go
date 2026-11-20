package sync

import (
	"encoding/json"
	"net/http"

	"github.com/NopeNix/Ferriage/internal/api"
	"github.com/NopeNix/Ferriage/internal/version"
)

// Handler serves the sync wire endpoints of ADR 0001.
type Handler struct {
	Store  *Store
	PeerID string
}

// NewHandler returns an http.Handler for /sync/manifest and
// /sync/blob, and /peer (peer identity for the UI).
func NewHandler(store *Store, peerID string) http.Handler {
	h := &Handler{Store: store, PeerID: peerID}
	mux := http.NewServeMux()
	mux.HandleFunc("/sync/manifest", h.manifest)
	mux.HandleFunc("/sync/blob", h.blob)
	mux.HandleFunc("/peer", h.peer)
	return mux
}

func (h *Handler) manifest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	m, err := h.Store.BuildManifest()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "manifest failed")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(m)
}

func (h *Handler) blob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	p := r.URL.Query().Get("path")
	data, err := h.Store.ReadBlob(p)
	if err != nil {
		writeErr(w, statusFor(err), err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", itoa(len(data)))
	_, _ = w.Write(data)
}

func (h *Handler) peer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(api.PeerResponse{
		PeerID:  h.PeerID,
		Version: version.Version,
		Commit:  version.Commit,
	})
}

func statusFor(err error) int {
	if e, ok := err.(*Error); ok {
		switch e.Code {
		case "bad_path", "hash_mismatch":
			return http.StatusBadRequest
		case "not_found":
			return http.StatusNotFound
		case "too_large":
			return http.StatusRequestEntityTooLarge
		}
	}
	return http.StatusInternalServerError
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(api.ErrorResponse{Error: msg})
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

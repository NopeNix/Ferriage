package audit

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/NopeNix/Ferriage/internal/api"
)

// Register serves GET /audit (newest first, filtered by
// limit/actor/action/since) on the peer mux. There is
// deliberately no write or delete route: the log is
// append-only by API design (ADR 0008).
func Register(mux *http.ServeMux, s *Store) {
	h := &Handler{Store: s}
	mux.HandleFunc("GET /audit", h.list)
}

// Handler serves the audit read surface.
type Handler struct {
	Store *Store
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := 50
	if v := strings.TrimSpace(q.Get("limit")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 500 {
			writeErr(w, http.StatusBadRequest, "limit must be 1..500")
			return
		}
		limit = n
	}
	var since time.Time
	if v := strings.TrimSpace(q.Get("since")); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "since must be RFC 3339")
			return
		}
		since = t
	}
	entries := h.Store.Query(Filter{
		Actor:  strings.TrimSpace(q.Get("actor")),
		Action: strings.TrimSpace(q.Get("action")),
		Since:  since,
		Limit:  limit,
	})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(api.AuditResponse{Entries: entries})
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(api.ErrorResponse{Error: msg})
}

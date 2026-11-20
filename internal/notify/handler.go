package notify

import (
	"encoding/json"
	"net/http"

	"github.com/NopeNix/Ferriage/internal/api"
)

// Register serves GET /notify (redacted status) and
// POST /notify/test (synchronous test send) on the peer mux.
// There is deliberately no PUT/POST for URLs: configuration is
// env-only (ADR 0007, option A).
func Register(mux *http.ServeMux, n *Notifier) {
	h := &Handler{Notifier: n}
	mux.HandleFunc("GET /notify", h.status)
	mux.HandleFunc("POST /notify/test", h.test)
}

// Handler exposes the notifier's redacted status and test send.
type Handler struct {
	Notifier *Notifier
}

func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	var st Status
	if h.Notifier != nil {
		st = h.Notifier.Status()
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(api.NotifyStatus{
		Enabled:   st.Enabled,
		Channels:  st.Channels,
		LastEvent: st.LastEvent,
		LastAt:    st.LastAt,
		LastErr:   st.LastErr,
	})
}

func (h *Handler) test(w http.ResponseWriter, r *http.Request) {
	if h.Notifier == nil || !h.Notifier.Enabled() {
		writeErr(w, http.StatusServiceUnavailable, ErrDisabled.Error())
		return
	}
	if err := h.Notifier.Test(r.Context()); err != nil {
		writeErr(w, http.StatusBadGateway, "test notification failed: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(api.NotifyTestResponse{Sent: true, Event: string(EventTest)})
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(api.ErrorResponse{Error: msg})
}

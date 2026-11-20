package onboard

import (
	"encoding/json"
	"net/http"

	"github.com/NopeNix/Ferriage/internal/api"
	"github.com/NopeNix/Ferriage/internal/notify"
)

// Register serves POST /hosts/onboard on the peer mux.
// n may be nil (notifications disabled).
func Register(mux *http.ServeMux, stateDir string, selfURL string, n *notify.Notifier) {
	h := &Handler{StateDir: stateDir, SelfURL: selfURL, Notifier: n}
	mux.HandleFunc("POST /hosts/onboard", h.onboard)
}

// Handler onboards hosts over SSH (ADR 0006).
type Handler struct {
	StateDir string
	// SelfURL is the URL the new peer pulls mesh state from
	// (this peer's own address as reachable from the new host).
	SelfURL  string
	Notifier *notify.Notifier
}

func (h *Handler) onboard(w http.ResponseWriter, r *http.Request) {
	var req Request
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request body")
		return
	}
	if req.PeerURL == "" {
		req.PeerURL = h.SelfURL
	}
	res, err := Onboard(r.Context(), &req, h.StateDir, nil, nil)
	// Best-effort notification (ADR 0007): never blocks or
	// fails onboarding; a nil Notifier is a no-op.
	subject := req.Address
	if err != nil {
		h.Notifier.Notify(r.Context(), notify.EventOnboardFailed, subject, err.Error())
	} else if res != nil && res.Healthy {
		h.Notifier.Notify(r.Context(), notify.EventOnboardOK, subject, "healthy")
	} else if res != nil {
		h.Notifier.Notify(r.Context(), notify.EventOnboardFailed, subject, "not healthy")
	}
	if err != nil {
		status := http.StatusInternalServerError
		if e, ok := err.(*Error); ok && e.Code == "bad_request" {
			status = http.StatusBadRequest
		}
		if res != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(toResponse(res, err.Error()))
			return
		}
		writeErr(w, status, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toResponse(res, ""))
}

func toResponse(r *Result, errMsg string) api.OnboardResponse {
	steps := make([]api.OnboardStep, 0, len(r.Steps))
	for _, s := range r.Steps {
		steps = append(steps, api.OnboardStep{Name: s.Name, OK: s.OK, Detail: s.Detail})
	}
	return api.OnboardResponse{
		Address:   r.Address,
		Docker:    r.Docker,
		Compose:   r.Compose,
		Container: r.Container,
		PeerURL:   r.PeerURL,
		Healthy:   r.Healthy,
		Steps:     steps,
		Error:     errMsg,
	}
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(api.ErrorResponse{Error: msg})
}

package deploy

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/NopeNix/Ferriage/internal/api"
	"github.com/NopeNix/Ferriage/internal/notify"
)

// Register serves POST /stacks/{name}/deploy on the peer mux.
// n may be nil (notifications disabled); Notify is a no-op then.
func Register(mux *http.ServeMux, stateDir string, n *notify.Notifier) {
	h := &Handler{StateDir: stateDir, Cfg: DefaultConfig(), Notifier: n}
	mux.HandleFunc("POST /stacks/{name}/deploy", h.deploy)
}

// Handler deploys stacks from the state directory.
type Handler struct {
	StateDir string
	Cfg      Config
	Notifier *notify.Notifier
}

func (h *Handler) deploy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
		writeErr(w, http.StatusBadRequest, "bad stack name")
		return
	}
	stackDir := filepath.Join(h.StateDir, name)
	compose := filepath.Join(stackDir, "compose.yaml")
	if _, err := os.Stat(compose); err != nil {
		writeErr(w, http.StatusNotFound, "stack not found")
		return
	}
	cfg := h.Cfg
	if cfg.Compose == "" {
		cfg.Compose = "docker"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 120 * time.Second
	}
	res, err := Deploy(r.Context(), stackDir, cfg)
	// Best-effort notification (ADR 0007): never blocks or
	// fails the deploy; a nil Notifier is a no-op.
	if err != nil {
		h.Notifier.Notify(r.Context(), notify.EventDeployFailed, name, err.Error())
	} else if res != nil && res.Healthy {
		h.Notifier.Notify(r.Context(), notify.EventDeployOK, name, "healthy")
	} else if res != nil {
		h.Notifier.Notify(r.Context(), notify.EventDeployFailed, name, res.Detail)
	}
	if err != nil {
		if res != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			res.Detail = err.Error()
			_ = json.NewEncoder(w).Encode(res)
			return
		}
		writeErr(w, statusFor(err), err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toResponse(res))
}

// toResponse maps deploy.Result onto the typed API contract.
func toResponse(r *Result) api.DeployResponse {
	svcs := make([]api.DeploySvc, 0, len(r.Services))
	for _, s := range r.Services {
		svcs = append(svcs, api.DeploySvc{
			Name:      s.Name,
			Health:    s.Health,
			HasHealth: s.HasHealth,
			Running:   s.Running,
		})
	}
	return api.DeployResponse{
		Stack:    r.Stack,
		Services: svcs,
		Healthy:  r.Healthy,
		Detail:   r.Detail,
	}
}

func statusFor(err error) int {
	if e, ok := err.(*Error); ok && e.Code == "bad_request" {
		return http.StatusBadGateway
	}
	return http.StatusInternalServerError
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(api.ErrorResponse{Error: msg})
}

package deploy

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Register serves POST /stacks/{name}/deploy on the peer mux.
func Register(mux *http.ServeMux, stateDir string) {
	h := &Handler{StateDir: stateDir, Cfg: DefaultConfig()}
	mux.HandleFunc("POST /stacks/{name}/deploy", h.deploy)
}

// Handler deploys stacks from the state directory.
type Handler struct {
	StateDir string
	Cfg      Config
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
	_ = json.NewEncoder(w).Encode(res)
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
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

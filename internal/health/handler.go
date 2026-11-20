// Package health serves the peer liveness probe.
package health

import (
	"encoding/json"
	"net/http"

	"github.com/NopeNix/Ferriage/internal/version"
)

// Response is the body of GET /healthz.
type Response struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	Commit  string `json:"commit"`
}

// NewHandler returns the health endpoint handler.
func NewHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(Response{
			Status:  "ok",
			Version: version.Version,
			Commit:  version.Commit,
		})
	})
}

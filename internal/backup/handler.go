package backup

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/NopeNix/Ferriage/internal/api"
	"github.com/NopeNix/Ferriage/internal/deploy"
	"github.com/NopeNix/Ferriage/internal/notify"
	"github.com/NopeNix/Ferriage/internal/snapshot"
)

// Register serves the backup surfaces (ADR 0011) on the peer mux:
//
//	GET  /backup              repo status + snapshot history
//	POST /backup/run           run now (body {"stack":"name"} or {"stack":""})
//	POST /backup/restore       restore + health-gated deploy (body {"stack":"x","snapshot":"id"})
//	POST /backup/forget        apply retention pruning
//
// All mutating routes are audited by middleware as backup.run,
// backup.restore, backup.forget. Password never crosses the API.
func Register(mux *http.ServeMux, stateDir string, n *notify.Notifier) {
	h := &Handler{StateDir: stateDir, Notifier: n, NewMgr: func() *Manager { return NewManager(stateDir) }}
	mux.HandleFunc("GET /backup", h.list)
	mux.HandleFunc("POST /backup/run", h.runBackup)
	mux.HandleFunc("POST /backup/restore", h.restore)
	mux.HandleFunc("POST /backup/forget", h.forget)
}

// Handler serves backup endpoints.
type Handler struct {
	StateDir string
	Notifier *notify.Notifier
	// NewMgr is overridable for tests.
	NewMgr func() *Manager
	// BackendFor builds the volume backend (snapshot.BackendFor in prod).
	BackendFor func() (volumeBackend, func(), error)
}

func (h *Handler) backend() (volumeBackend, func(), error) {
	if h.BackendFor != nil {
		return h.BackendFor()
	}
	b, done, err := snapshot.BackendFor()
	if err != nil {
		return nil, func() {}, err
	}
	return b, done, nil
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	out, err := h.NewMgr().List(r.Context())
	if err != nil {
		writeErr(w, statusFor(err), err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toList(out))
}

func (h *Handler) runBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req api.BackupRunRequest
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}
	stack := stringsTrim(req.Stack)
	vols, done, err := h.backend()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer done()
	res, err := h.NewMgr().Run(r.Context(), stack, vols)
	if err != nil {
		h.Notifier.Notify(r.Context(), notify.EventBackupFailed, stackName(stack), err.Error())
		writeErr(w, statusFor(err), err.Error())
		return
	}
	h.Notifier.Notify(r.Context(), notify.EventBackupOK, stackName(stack), "snapshot "+res.Snapshot)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toResult(res))
}

func (h *Handler) restore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req api.BackupRestoreRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad restore request")
		return
	}
	stack := stringsTrim(req.Stack)
	if stack == "" {
		writeErr(w, http.StatusBadRequest, "restore needs a stack name (use snapshot id of a node backup for stacks)")
		return
	}
	vols, done, err := h.backend()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer done()
	deployFn := func(ctx_ context.Context, s string) (bool, string, error) {
		res, err := deploy.Deploy(ctx_, filepath.Join(h.StateDir, s), deploy.DefaultConfig())
		if err != nil {
			if res != nil {
				return false, res.Detail, err
			}
			return false, "", err
		}
		return res.Healthy, res.Detail, nil
	}
	res, err := h.NewMgr().Restore(r.Context(), stack, stringsTrim(req.Snapshot), vols, deployFn)
	if err != nil {
		h.Notifier.Notify(r.Context(), notify.EventBackupFailed, stackName(stack), err.Error())
		writeErr(w, statusFor(err), err.Error())
		return
	}
	h.Notifier.Notify(r.Context(), notify.EventBackupOK, stackName(stack), "restored "+res.Snapshot+" healthy="+fmtBool(res.Healthy))
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toResult(res))
}

func (h *Handler) forget(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	res, err := h.NewMgr().Forget(r.Context())
	if err != nil {
		writeErr(w, statusFor(err), err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toResult(res))
}

func stackName(s string) string {
	if s == "" {
		return "node"
	}
	return s
}

func fmtBool(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func stringsTrim(s string) string {
	return strings.TrimSpace(s)
}

func toList(l *List) api.BackupList {
	snaps := make([]api.BackupSnapshot, 0, len(l.Snapshots))
	for _, s := range l.Snapshots {
		snaps = append(snaps, api.BackupSnapshot{
			ID:      s.ID,
			ShortID: s.ShortID,
			Time:    s.Time,
			Tags:    s.Tags,
			Paths:   s.Paths,
			Size:    int64(s.Size),
		})
	}
	return api.BackupList{
		Status: api.BackupStatus{
			Repo:         l.Status.Repo,
			Initialized:  l.Status.Initialized,
			Snapshots:    l.Status.Snapshots,
			PasswordSet:  l.Status.PasswordSet,
			Restic:       l.Status.Restic,
			LastBackupAt: l.Status.LastBackupAt,
			LastBackupOK: l.Status.LastBackupOK,
			LastError:    l.Status.LastError,
		},
		Snapshots: snaps,
	}
}

func toResult(r *Result) api.BackupResult {
	return api.BackupResult{
		Scope:    r.Scope,
		Stack:    r.Stack,
		Snapshot: r.Snapshot,
		Healthy:  r.Healthy,
		Detail:   r.Detail,
		Pruned:   r.Pruned,
	}
}

func statusFor(err error) int {
	if e, ok := err.(*Error); ok {
		switch e.Code {
		case "bad_request":
			return http.StatusBadRequest
		case "not_found":
			return http.StatusNotFound
		case "upstream":
			return http.StatusBadGateway
		}
	}
	return http.StatusInternalServerError
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(api.ErrorResponse{Error: msg})
}

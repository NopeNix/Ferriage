package update

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/NopeNix/Ferriage/internal/api"
)

// Register serves the auto-update surfaces (ADR 0009) on the peer
// mux: GET /update, GET /update/{name}, POST /update/{name}/check,
// PUT /update/{name}/policy, POST /update/{name}/run.
func Register(mux *http.ServeMux, m *Manager) {
	r := &handler{m: m}
	mux.HandleFunc("GET /update", r.list)
	mux.HandleFunc("GET /update/{name}", r.one)
	mux.HandleFunc("POST /update/{name}/check", r.check)
	mux.HandleFunc("PUT /update/{name}/policy", r.setPolicy)
	mux.HandleFunc("POST /update/{name}/run", r.run)
}

type handler struct {
	m *Manager
}

func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	h.m.SeedFromDisk()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(api.UpdateList{Stacks: h.m.Reports()})
}

func (h *handler) one(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !validName(name) {
		writeErr(w, http.StatusBadRequest, "bad stack name")
		return
	}
	h.m.SeedFromDisk()
	st := h.m.Report(name)
	if st == nil {
		writeErr(w, http.StatusNotFound, "stack not found")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(st)
}

// check forces one synchronous check for a stack (reads the
// running containers, compares digests, records the report).
// It never deploys, even for `auto` stacks — that is /run.
// Like GETs it is not audited (ADR 0009: a probe, not an action).
func (h *handler) check(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !validName(name) {
		writeErr(w, http.StatusBadRequest, "bad stack name")
		return
	}
	stacks, err := h.m.Peers.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	var found *adoptedStack
	for i := range stacks {
		if stacks[i].Name == name {
			found = &stacks[i]
			break
		}
	}
	if found == nil {
		if !stackExists(h.m.StateDir, name) {
			writeErr(w, http.StatusNotFound, "stack not found")
			return
		}
		// On disk but not running: the check still
		// records a not-running report.
		found = &adoptedStack{Name: name}
	}
	h.m.check(r.Context(), *found, nowUTC())
	st := h.m.Report(name)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(st)
}

// setPolicy writes one stack's policy override. Clears the
// "available" state so the next check re-evaluates under the new
// policy. Audited via the middleware (update.policy).
func (h *handler) setPolicy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !validName(name) {
		writeErr(w, http.StatusBadRequest, "bad stack name")
		return
	}
	if !stackExists(h.m.StateDir, name) {
		writeErr(w, http.StatusNotFound, "stack not found")
		return
	}
	var req api.UpdatePolicyRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	if err := dec.Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad policy request")
		return
	}
	req.Policy = strings.TrimSpace(req.Policy)
	req.Interval = strings.TrimSpace(req.Interval)
	if req.Policy == "" && req.Interval == "" {
		writeErr(w, http.StatusBadRequest, "nothing to set (policy and interval both empty)")
		return
	}
	if req.Policy != "" && !validPolicy(req.Policy) {
		writeErr(w, http.StatusBadRequest, "invalid policy (want poll or auto)")
		return
	}
	sp, err := h.m.Files.set(name, req.Policy, req.Interval)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	// Re-evaluate the report under the new policy.
	stateful := isStateful(r.Context(), h.m.StateDir, name)
	ov := PolicyOverride{}
	if sp.Policy != "" {
		ov.Policy = sp.Policy
		ov.HasPolicy = true
	}
	if sp.Interval != "" {
		ov.Interval, _ = parseDur(sp.Interval)
		ov.HasInterval = true
	}
	policy, interval := EffectivePolicy(h.m.Cfg, ov, stateful)
	h.m.mu.Lock()
	if rep, ok := h.m.reports[name]; ok && rep != nil {
		rep.mu.Lock()
		rep.policy = policy
		rep.stateful = stateful
		rep.mu.Unlock()
	}
	h.m.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(api.UpdatePolicyResponse{
		Stack:    name,
		Policy:   policy,
		Interval: interval.String(),
		Stateful: stateful,
	})
}

// run forces the snapshot-first auto-update flow now, regardless
// of policy. Audited via the middleware (update.run).
func (h *handler) run(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !validName(name) {
		writeErr(w, http.StatusBadRequest, "bad stack name")
		return
	}
	if !stackExists(h.m.StateDir, name) {
		writeErr(w, http.StatusNotFound, "stack not found")
		return
	}
	run := h.m.run(r.Context(), name, "manual run")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(run)
}

func validName(s string) bool {
	if s == "" || s == "." || s == ".." {
		return false
	}
	for _, c := range s {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_', c == '.':
		default:
			return false
		}
	}
	return true
}

func stackExists(stateDir, name string) bool {
	// State dir listing (not Docker): a stack dir is the
	// unit of management, running or not.
	entries, err := readDirNames(stateDir)
	if err != nil {
		return false
	}
	for _, n := range entries {
		if n == name {
			return true
		}
	}
	return false
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(api.ErrorResponse{Error: msg})
}

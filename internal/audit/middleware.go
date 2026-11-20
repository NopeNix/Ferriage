package audit

import (
	"log"
	"net/http"
	"strings"
)

// Middleware wraps the peer mux and appends one audit entry
// per mutating request on the audited routes. It captures the
// actor (X-Ferriage-Actor, default "unknown"), the action and
// target derived from the route, and the HTTP status the
// handler produced. Reads (GET/HEAD/OPTIONS) and mesh
// machinery (/sync/*, /healthz, /openapi*) are never recorded.
// Recording is best-effort: a failed append logs to stderr
// and never fails the request.
func Middleware(s *Store, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action, target, ok := classify(r)
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if err := s.Append(Entry{
			Actor:  actorOf(r),
			Action: string(action),
			Target: target,
			Status: rec.status,
		}); err != nil {
			// Best-effort by contract (ADR 0008): never fail
			// the request because the audit file is broken.
			log.Printf("ferriage audit: append failed: %v", err)
		}
	})
}

// statusRecorder captures the first WriteHeader status.
type statusRecorder struct {
	http.ResponseWriter
	status  int
	written bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.written {
		s.status = code
		s.written = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if !s.written {
		s.status = http.StatusOK
		s.written = true
	}
	return s.ResponseWriter.Write(b)
}

func actorOf(r *http.Request) string {
	if v := strings.TrimSpace(r.Header.Get(ActorHeader)); v != "" {
		return truncate(v, 128)
	}
	return "unknown"
}

// classify maps a request to its audit action and target.
// The second return is the target (stack or host name). The
// route patterns mirror the mux registrations; unknown
// mutating routes are still recorded with a generic action
// so no operator write escapes the log.
func classify(r *http.Request) (Action, string, bool) {
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
		return "", "", false
	}
	p := r.URL.Path
	switch {
	case p == "/hosts/onboard":
		// The onboard target (address) lives in the request
		// body, which the middleware must not consume — the
		// handler reads it. The address is already in the
		// peer's stdout log; the entry records the action
		// with a generic target (ADR 0008: no secrets, and
		// no body peeking).
		return ActionOnboard, "onboard", true
	case p == "/notify/test":
		return ActionNotifyTest, "", true
	case strings.HasPrefix(p, "/update/"):
		// /update/{name}/policy | /update/{name}/run (ADR 0009).
		// /update/{name}/check is a probe, not an action: it is
		// POST but changes no durable state (only the transient
		// report cache), so it is not audited — like GETs.
		rest := strings.TrimPrefix(p, "/update/")
		parts := strings.SplitN(rest, "/", 2)
		if len(parts) < 2 {
			return "", "", false
		}
		if parts[1] == "policy" {
			return ActionUpdatePolicy, parts[0], true
		}
		if parts[1] == "run" {
			return ActionUpdateRun, parts[0], true
		}
		if parts[1] == "check" {
			return "", "", false
		}
		return Action(p), parts[0], true
	case strings.HasPrefix(p, "/stacks/"):
		rest := strings.TrimPrefix(p, "/stacks/")
		// rest = "{name}/deploy" | "{name}/export" | ...
		parts := strings.SplitN(rest, "/", 3)
		name := parts[0]
		if len(parts) < 2 {
			return "", "", false
		}
		switch parts[1] {
		case "deploy":
			return ActionDeploy, name, true
		case "export":
			return ActionExport, name, true
		case "import":
			return ActionImport, name, true
		case "compose":
			if r.Method == http.MethodPut {
				return ActionComposeSave, name, true
			}
			return "", "", false
		case "services":
			if len(parts) == 3 && strings.HasSuffix(parts[2], "/exec") {
				svc := strings.TrimSuffix(parts[2], "/exec")
				return ActionExec, name + "/" + svc, true
			}
			return "", "", false
		}
	}
	// Unknown mutating routes: record generically so an
	// operator write can never silently escape the log.
	return Action(p), p, true
}

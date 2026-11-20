// Package audit is the peer's operator audit log (CONCEPT
// should-have: who did what, when; ADR 0008). It records
// mutating operator actions as append-only JSON lines under
// "<stateDir>/.ferriage/audit.jsonl" — next to identity and the
// sync ledger, so the sync walker (which never walks .ferriage/)
// can never replicate or delete it. The log is a derived
// record, not state: losing it loses history, never stacks.
//
// Recording is middleware-based (see Middleware): the wrapper
// captures actor, action, target, and HTTP status from the
// request/response pair, so handlers stay unchanged and the
// actor can never be spoofed by a handler body. Reads (GET) and
// mesh machinery (/sync/*, health, openapi) are never recorded.
//
// The log is bounded by line count (default 10 000) and
// best-effort: an unwritable file logs to stderr and the
// operation still proceeds. There is no write or delete API —
// append-only by design, GET /audit is the only surface.
package audit

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/NopeNix/Ferriage/internal/api"
	"github.com/NopeNix/Ferriage/internal/identity"
)

// Action is one audited operator action (closed set).
type Action string

const (
	ActionDeploy      Action = "deploy"
	ActionExport      Action = "export"
	ActionImport      Action = "import"
	ActionComposeSave Action = "compose.save"
	ActionExec        Action = "exec"
	ActionOnboard     Action = "onboard"
	ActionNotifyTest  Action = "notify.test"
)

// ActorHeader carries the reported operator identity. The BFF
// sets it from the verified OIDC session (ADR 0004); the peer
// is plaintext on the trusted net, so the header is a record,
// not an authentication boundary.
const ActorHeader = "X-Ferriage-Actor"

// DefaultMaxLines caps the audit file (FERRIAGE_AUDIT_MAX_LINES).
const DefaultMaxLines = 10000

// MaxDetail caps the detail field so one chatty handler cannot
// bloat the log.
const MaxDetail = 200

// Entry is one audit line.
type Entry struct {
	Ts     string `json:"ts"`
	Actor  string `json:"actor"`
	Action string `json:"action"`
	Target string `json:"target,omitempty"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// Filter selects entries for a query.
type Filter struct {
	Actor  string
	Action string
	Since  time.Time // zero = no lower bound
	Limit  int       // <=0 = default
}

// Store appends and reads audit entries for one state dir.
// The zero value is usable only via New (it needs a path).
type Store struct {
	path     string
	maxLines int
	disabled bool
	mu       sync.Mutex
}

// Disabled reports whether recording is turned off
// (FERRIAGE_AUDIT_DISABLE=1).
func (s *Store) Disabled() bool { return s != nil && s.disabled }

// New opens the audit store for a state dir. The file is
// created lazily on first append.
func New(stateDir string, maxLines int, disabled bool) *Store {
	if maxLines <= 0 {
		maxLines = DefaultMaxLines
	}
	dir := filepath.Join(stateDir, identity.DirName)
	return &Store{
		path:     filepath.Join(dir, "audit.jsonl"),
		maxLines: maxLines,
		disabled: disabled,
	}
}

// Append writes one entry. Best-effort: errors are returned for
// the caller to log — the middleware never fails a request
// because the audit file is unwritable.
func (s *Store) Append(e Entry) error {
	if s == nil || s.disabled {
		return nil
	}
	if e.Ts == "" {
		e.Ts = time.Now().UTC().Format(time.RFC3339)
	}
	if e.Actor == "" {
		e.Actor = "unknown"
	}
	e.Detail = truncate(e.Detail, MaxDetail)
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	// Bound the file: count lines cheaply by size proxy first —
	// if the file is well under maxLines*~300 bytes it cannot
	// be over the cap. Only then count and rewrite.
	if st, err := os.Stat(s.path); err == nil && st.Size() > int64(s.maxLines*64) {
		if err := s.pruneLocked(); err != nil {
			return err
		}
	}
	return nil
}

// pruneLocked rewrites the file keeping only the newest
// maxLines lines. Caller must hold mu.
func (s *Store) pruneLocked() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) <= s.maxLines {
		return nil
	}
	keep := lines[len(lines)-s.maxLines:]
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".audit-tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.WriteString(strings.Join(keep, "\n") + "\n"); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, s.path)
}

// Query returns entries matching the filter, newest first.
func (s *Store) Query(f Filter) []api.AuditEntry {
	if s == nil {
		return []api.AuditEntry{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []api.AuditEntry{}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return out
	}
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	var matched []api.AuditEntry
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e api.AuditEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		if f.Actor != "" && e.Actor != f.Actor {
			continue
		}
		if f.Action != "" && e.Action != f.Action {
			continue
		}
		if !f.Since.IsZero() {
			t, err := time.Parse(time.RFC3339, e.Ts)
			if err != nil || t.Before(f.Since) {
				continue
			}
		}
		matched = append(matched, e)
	}
	// Newest first: the file is append-ordered, so reverse.
	sort.SliceStable(matched, func(i, j int) bool { return matched[i].Ts > matched[j].Ts })
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	if len(matched) > limit {
		matched = matched[:limit]
	}
	if matched == nil {
		matched = []api.AuditEntry{}
	}
	return matched
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

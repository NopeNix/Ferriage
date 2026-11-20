// Package notify sends best-effort Shoutrrr notifications for
// Ferriage lifecycle events (CONCEPT: Shoutrrr, the built-in
// channel — Slack, Discord, Telegram, email, webhooks, generic
// URLs). Option A (ADR 0007): URLs come only from the environment
// (FERRIAGE_NOTIFY_URLS, comma-separated); there is no endpoint
// that accepts or returns the URLs, so the secret never crosses
// the API in either direction.
//
// Notifications are strictly best-effort: a dead webhook never
// blocks or fails a deploy/onboard — sends run in a goroutine
// bounded by Timeout (and ctx, when set), and failures are
// logged, not returned. The last send outcome is kept for
// GET /notify status (URLs and message bodies are never exposed).
package notify

import (
	"context"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/containrrr/shoutrrr"
	"github.com/containrrr/shoutrrr/pkg/types"
)

// PeerIDFunc returns the local peer id for message prefixing.
// May be nil (no prefix beyond the message itself).
type PeerIDFunc func() string

// Config tunes the notifier.
type Config struct {
	// URLs are the raw Shoutrrr service URLs (never logged).
	URLs []string
	// Timeout bounds one send attempt (default 10s).
	Timeout time.Duration
	// PeerID prefixes every message (default: none).
	PeerID PeerIDFunc
}

// URLsFromEnv parses FERRIAGE_NOTIFY_URLS (comma-separated).
func URLsFromEnv() []string {
	return ParseURLs(os.Getenv("FERRIAGE_NOTIFY_URLS"))
}

// ParseURLs splits a comma-separated URL list, dropping blanks.
// Shoutrrr URLs contain no commas (credentials are in the URL's
// userinfo), so a plain comma split is safe.
func ParseURLs(v string) []string {
	var out []string
	for _, u := range strings.Split(v, ",") {
		if u = strings.TrimSpace(u); u != "" {
			out = append(out, u)
		}
	}
	return out
}

// Event kinds emitted by the peer (closed set; the UI and CLI
// surface them so operators can preview what a notification
// looks like).
type Event string

const (
	EventDeployOK      Event = "deploy.ok"
	EventDeployFailed  Event = "deploy.failed"
	EventOnboardOK     Event = "onboard.ok"
	EventOnboardFailed Event = "onboard.failed"
	EventTest          Event = "notify.test"
	EventPeerStarted   Event = "peer.started"
	// Auto-update events (ADR 0009): a newer digest is an event
	// whether or not the peer redeploys it.
	EventUpdateAvailable    Event = "update.available"
	EventUpdateOK           Event = "update.ok"
	EventUpdateFailed       Event = "update.failed"
	EventUpdateSnapshotFail Event = "update.snapshot_failed"
	// Backup events (ADR 0011).
	EventBackupOK     Event = "backup.ok"
	EventBackupFailed Event = "backup.failed"
)

// message is one notification: an event, its subject (stack or
// host), and a one-line detail.
type message struct {
	Event   Event
	Subject string
	Detail  string
}

func (m message) text() string {
	s := "Ferriage " + string(m.Event)
	if m.Subject != "" {
		s += " " + m.Subject
	}
	if m.Detail != "" {
		s += ": " + m.Detail
	}
	return s
}

// Status is the GET /notify body (no URLs, no message bodies).
type Status struct {
	Enabled   bool   `json:"enabled"`
	Channels  int    `json:"channels"`
	LastEvent string `json:"last_event,omitempty"`
	LastAt    string `json:"last_at,omitempty"`
	LastErr   string `json:"last_error,omitempty"`
}

// Notifier is the peer's Shoutrrr sender. The zero value is
// disabled; build with New.
type Notifier struct {
	cfg   Config
	send  func(message string) error
	mu    sync.Mutex
	last  Status
	valid bool
}

// New builds a Notifier from raw Shoutrrr URLs. Construction
// never fails the peer: invalid URLs are logged (count only,
// never the URL) and skipped, and a nil sender is replaced by
// an erroring stub so Notify stays a safe no-op path.
func New(urls []string, peerID PeerIDFunc) (*Notifier, error) {
	cfg := Config{URLs: urls, Timeout: 10 * time.Second, PeerID: peerID}
	n := &Notifier{cfg: cfg}
	var invalid int
	for _, u := range urls {
		if _, err := shoutrrr.CreateSender(u); err != nil {
			invalid++
		}
	}
	if invalid > 0 {
		log.Printf("ferriage notify: %d invalid URL(s) ignored (check FERRIAGE_NOTIFY_URLS)", invalid)
	}
	n.send = func(msg string) error {
		sender, err := shoutrrr.CreateSender(cfg.URLs...)
		if err != nil {
			return err
		}
		for _, e := range sender.Send(msg, &types.Params{}) {
			if e != nil {
				return e
			}
		}
		return nil
	}
	return n, nil
}

// Enabled reports whether at least one channel is configured.
func (n *Notifier) Enabled() bool { return n != nil && len(n.cfg.URLs) > 0 }

// Channels returns the number of configured channels.
func (n *Notifier) Channels() int {
	if n == nil {
		return 0
	}
	return len(n.cfg.URLs)
}

// Status returns the redacted status for GET /notify.
func (n *Notifier) Status() Status {
	if n == nil {
		return Status{}
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	st := n.last
	st.Enabled = n.Enabled()
	st.Channels = n.Channels()
	return st
}

// prefixed builds the wire message with the peer id prefix.
func (n *Notifier) prefixed(m message) message {
	if n.cfg.PeerID != nil {
		if id := n.cfg.PeerID(); id != "" {
			m.Detail = strings.TrimSpace("peer " + shortID(id) + " " + m.Detail)
		}
	}
	return m
}

// Notify sends one event. It never blocks the caller on the
// network: the send runs in a goroutine bounded by Timeout, and
// the result is recorded for status. The caller's ctx is
// deliberately NOT propagated: handlers pass the request ctx,
// which is canceled when the response returns — after the send
// already started — and a canceled context would be recorded as
// a delivery error even though the webhook got the message.
// A nil or disabled Notifier is a no-op so handlers need no
// nil checks.
func (n *Notifier) Notify(_ context.Context, ev Event, subject, detail string) {
	if !n.Enabled() {
		return
	}
	m := n.prefixed(message{Event: ev, Subject: subject, Detail: detail})
	go n.deliver(m)
}

func (n *Notifier) deliver(m message) {
	err := n.sendSync(context.Background(), m)
	n.mu.Lock()
	n.last.LastEvent = string(m.Event)
	n.last.LastAt = time.Now().UTC().Format(time.RFC3339)
	n.last.LastErr = ""
	if err != nil {
		n.last.LastErr = err.Error()
	}
	n.valid = true
	n.mu.Unlock()
	if err != nil {
		log.Printf("ferriage notify: %s failed: %v", m.Event, err)
	} else {
		log.Printf("ferriage notify: %s sent", m.Event)
	}
}

// sendSync performs one bounded send and returns its error.
func (n *Notifier) sendSync(ctx context.Context, m message) error {
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := n.cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- n.send(m.text()) }()
	select {
	case err := <-done:
		return err
	case <-cctx.Done():
		return cctx.Err()
	}
}

// Test sends a synchronous test notification and returns its
// outcome (used by POST /notify/test). Disabled → ErrDisabled,
// which the handler maps to 503.
func (n *Notifier) Test(ctx context.Context) error {
	if !n.Enabled() {
		return ErrDisabled
	}
	m := n.prefixed(message{Event: EventTest, Subject: "ping", Detail: "Ferriage notifications are wired"})
	err := n.sendSync(ctx, m)
	n.mu.Lock()
	n.last.LastEvent = string(EventTest)
	n.last.LastAt = time.Now().UTC().Format(time.RFC3339)
	n.last.LastErr = ""
	if err != nil {
		n.last.LastErr = err.Error()
	}
	n.valid = true
	n.mu.Unlock()
	return err
}

// ErrDisabled is returned by Test when no channels are configured.
var ErrDisabled = errDisabled("notifications disabled: no FERRIAGE_NOTIFY_URLS")

type errDisabled string

func (e errDisabled) Error() string { return string(e) }

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

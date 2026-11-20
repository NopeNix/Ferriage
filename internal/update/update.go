// Package update implements auto-update for Ferriage (CONCEPT
// § "Auto-update (distributed)"; ADR 0009). Each peer runs its own
// update loop: per-stack digest polling against the registry,
// `poll` (report only) vs `auto` (snapshot + health-gated
// redeploy), Shoutrrr on every event. No control plane — the peer
// that runs the stack is the peer that updates it.
//
// Policy lives in files, not a database: `.ferriage/update.json`
// under the state dir (never synced — `.ferriage/` is never
// walked by sync), the same rule as identity and the audit log.
// The loop is best-effort: a registry outage is logged and
// recorded, never an error to the operator, and never blocks the
// peer's other work.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/NopeNix/Ferriage/internal/api"
	"github.com/NopeNix/Ferriage/internal/identity"
)

// Policy values (closed set).
const (
	PolicyPoll = "poll"
	PolicyAuto = "auto"
)

// Service check outcomes (closed set).
const (
	OutcomeUpToDate     = "up-to-date"
	OutcomeAvailable    = "available"
	OutcomeUnresolvable = "unresolvable"
)

// Stack report states (closed set).
const (
	StateUpToDate     = "up-to-date"
	StateAvailable    = "available"
	StateUnresolvable = "unresolvable"
	StateNotRunning   = "not-running"
	StateCheckFailed  = "check-failed"
)

// validPolicy reports whether p is a known policy.
func validPolicy(p string) bool { return p == PolicyPoll || p == PolicyAuto }

// StackPolicy is one stack's entry in the policy file.
type StackPolicy struct {
	Policy   string `json:"policy"`
	Interval string `json:"interval,omitempty"`
}

// policyFile is the on-disk shape of .ferriage/update.json.
type policyFile struct {
	Stacks map[string]StackPolicy `json:"stacks"`
}

// PolicyOverride is the validated per-stack override derived
// from the policy file for one stack.
type PolicyOverride struct {
	Policy      string
	Interval    time.Duration
	HasPolicy   bool
	HasInterval bool
}

// Config tunes the updater.
type Config struct {
	// DefaultPolicy for stacks with no file entry ("poll").
	DefaultPolicy string
	// DefaultInterval for stacks with no file entry (24h).
	DefaultInterval time.Duration
	// Tick is the background loop's wakeup cadence (1h). A
	// stack's own interval still governs its check schedule;
	// the tick is only a wakeup, so intervals below Tick are
	// honored through last-check tracking.
	Tick time.Duration
	// Disabled turns the loop off (FERRIAGE_UPDATE_DISABLE=1);
	// API/CLI surfaces keep working.
	Disabled bool
}

// FileBackend reads and writes the policy file.
type FileBackend struct {
	path string
}

// NewFileBackend returns the policy-file backend for a state dir.
func NewFileBackend(stateDir string) *FileBackend {
	return &FileBackend{path: filepath.Join(stateDir, identity.DirName, "update.json")}
}

// load reads the policy file; a missing file is an empty policy set.
func (b *FileBackend) load() (policyFile, error) {
	var pf policyFile
	data, err := os.ReadFile(b.path)
	if err != nil {
		if os.IsNotExist(err) {
			pf.Stacks = map[string]StackPolicy{}
			return pf, nil
		}
		return pf, err
	}
	if err := json.Unmarshal(data, &pf); err != nil {
		return pf, fmt.Errorf("update: invalid policy file: %w", err)
	}
	if pf.Stacks == nil {
		pf.Stacks = map[string]StackPolicy{}
	}
	return pf, nil
}

// save writes the policy file atomically (temp file + rename, the
// sync compose-write pattern).
func (b *FileBackend) save(pf policyFile) error {
	if pf.Stacks == nil {
		pf.Stacks = map[string]StackPolicy{}
	}
	data, err := json.MarshalIndent(pf, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(b.path), 0o755); err != nil {
		return err
	}
	tmp := b.path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, b.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// get returns the validated override for one stack.
func (b *FileBackend) get(stack string) (PolicyOverride, error) {
	pf, err := b.load()
	if err != nil {
		return PolicyOverride{}, err
	}
	sp, ok := pf.Stacks[stack]
	if !ok {
		return PolicyOverride{}, nil
	}
	var ov PolicyOverride
	if sp.Policy != "" {
		if !validPolicy(sp.Policy) {
			return PolicyOverride{}, fmt.Errorf("update: invalid policy %q for %s", sp.Policy, stack)
		}
		ov.Policy = sp.Policy
		ov.HasPolicy = true
	}
	if sp.Interval != "" {
		d, err := time.ParseDuration(sp.Interval)
		if err != nil || d <= 0 {
			return PolicyOverride{}, fmt.Errorf("update: invalid interval %q for %s", sp.Interval, stack)
		}
		ov.Interval = d
		ov.HasInterval = true
	}
	return ov, nil
}

// set writes one stack's override. Empty policy preserves the
// existing policy; empty interval clears the interval override
// (back to the global default). The whole map is rewritten so a
// corrupt file never silently drops another stack's policy.
func (b *FileBackend) set(stack, policy, interval string) (StackPolicy, error) {
	pf, err := b.load()
	if err != nil {
		return StackPolicy{}, err
	}
	cur := pf.Stacks[stack]
	if policy != "" {
		if !validPolicy(policy) {
			return StackPolicy{}, fmt.Errorf("invalid policy %q (want poll or auto)", policy)
		}
		cur.Policy = policy
	}
	if interval != "" {
		d, err := time.ParseDuration(interval)
		if err != nil || d <= 0 {
			return StackPolicy{}, fmt.Errorf("invalid interval %q (want a Go duration like 24h)", interval)
		}
		cur.Interval = d.String()
	}
	if cur.Policy == "" && cur.Interval == "" {
		delete(pf.Stacks, stack)
		if err := b.save(pf); err != nil {
			return StackPolicy{}, err
		}
		return StackPolicy{}, nil
	}
	pf.Stacks[stack] = cur
	if err := b.save(pf); err != nil {
		return StackPolicy{}, err
	}
	return cur, nil
}

// EffectivePolicy is the policy a stack runs under: the file
// override wins; otherwise the global default — except stateful
// stacks (named volumes), which never inherit `auto` implicitly
// (ADR 0009: databases default to poll, never auto).
func EffectivePolicy(cfg Config, ov PolicyOverride, stateful bool) (policy string, interval time.Duration) {
	policy = cfg.DefaultPolicy
	if policy == "" {
		policy = PolicyPoll
	}
	interval = cfg.DefaultInterval
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	if ov.HasPolicy {
		policy = ov.Policy
	} else if stateful && policy == PolicyAuto {
		policy = PolicyPoll
	}
	if ov.HasInterval {
		interval = ov.Interval
	}
	return policy, interval
}

// serviceReport is one service's digest check outcome.
type serviceReport = api.UpdateService

// report is one stack's latest update report.
type report struct {
	mu        sync.Mutex
	stack     string
	state     string
	policy    string
	stateful  bool
	checkedAt time.Time
	services  []serviceReport
	lastRun   *api.UpdateRun
	snapshot  string
	err       string
}

// Manager is the peer's update loop and report cache.
type Manager struct {
	StateDir string
	Cfg      Config
	Files    *FileBackend
	Peers    PeerLister
	Digests  DigestChecker
	Updater  DeployAndSnapshot
	Notifier Notifier
	Audit    AuditSink

	mu      sync.Mutex
	reports map[string]*report
	last    map[string]time.Time
}

// PeerLister supplies the adopted stacks (running containers).
type PeerLister interface {
	List(context.Context) ([]adoptedStack, error)
}

// adoptedStack is the subset of adopt.Stack the updater needs.
type adoptedStack struct {
	Name     string
	Services []adoptedService
}

// adoptedService is one running service's image reference.
type adoptedService struct {
	Name  string
	Image string
}

// DigestChecker resolves local and remote digests for one image.
type DigestChecker interface {
	Check(ctx context.Context, imageRef string) (local, remote string, err error)
}

// DeployAndSnapshot performs the auto-update's snapshot + deploy.
type DeployAndSnapshot interface {
	Snapshot(ctx context.Context, stack string) (snapshotPath string, err error)
	Deploy(ctx context.Context, stack string) (healthy bool, detail string, err error)
}

// Notifier is the subset of *notify.Notifier the updater uses.
type Notifier interface {
	Notify(ctx context.Context, ev Event, subject, detail string)
}

// Event is the notifier event kind (stringly-typed to avoid an
// import cycle with internal/notify; the wire values are the
// notify.Event constants).
type Event string

const (
	EventUpdateAvailable    Event = "update.available"
	EventUpdateOK           Event = "update.ok"
	EventUpdateFailed       Event = "update.failed"
	EventUpdateSnapshotFail Event = "update.snapshot_failed"
)

// AuditSink records machine actions (actor "peer").
type AuditSink interface {
	Append(actor, action, target string, status int, detail string)
}

// NewManager builds a Manager with production defaults.
func NewManager(stateDir string, cfg Config, peers PeerLister, digests DigestChecker, du DeployAndSnapshot, n Notifier, a AuditSink) *Manager {
	return &Manager{
		StateDir: stateDir,
		Cfg:      cfg,
		Files:    NewFileBackend(stateDir),
		Peers:    peers,
		Digests:  digests,
		Updater:  du,
		Notifier: n,
		Audit:    a,
		reports:  map[string]*report{},
		last:     map[string]time.Time{},
	}
}

// Loop is the background update loop. It wakes every Tick and
// checks every stack whose interval has elapsed. ctx cancellation
// stops the loop.
func (m *Manager) Loop(ctx context.Context) {
	if m.Cfg.Disabled {
		log.Printf("ferriage update: loop disabled (FERRIAGE_UPDATE_DISABLE=1)")
		return
	}
	tick := m.Cfg.Tick
	if tick <= 0 {
		tick = time.Hour
	}
	log.Printf("ferriage update: loop started (tick %s, default interval %s, default policy %s)", tick, m.Cfg.DefaultInterval, m.Cfg.DefaultPolicy)
	t := time.NewTicker(tick)
	defer t.Stop()
	// First pass on startup so reports are warm for GET /update.
	m.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.tick(ctx)
		}
	}
}

func (m *Manager) tick(ctx context.Context) {
	stacks, err := m.Peers.List(ctx)
	if err != nil {
		log.Printf("ferriage update: list stacks: %v", err)
		return
	}
	now := time.Now()
	for _, s := range stacks {
		should, _, _ := m.due(s.Name, now)
		if !should {
			continue
		}
		m.check(ctx, s, now)
	}
}

// due reports whether stack's interval has elapsed since its last
// check. A never-checked stack is always due.
func (m *Manager) due(stack string, now time.Time) (due bool, _, _ time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	last, ok := m.last[stack]
	if !ok {
		return true, 0, 0
	}
	_ = last
	return now.Sub(last) >= m.intervalLocked(stack), 0, 0
}

// intervalLocked returns the effective interval for stack (caller
// holds mu). It re-reads the file override so a policy change
// takes effect on the next tick without a restart.
func (m *Manager) intervalLocked(stack string) time.Duration {
	ov, err := m.Files.get(stack)
	if err != nil {
		return m.Cfg.DefaultInterval
	}
	_, iv := EffectivePolicy(m.Cfg, ov, false)
	return iv
}

// check performs one digest check for one stack and, for `auto`
// policy stacks with a newer digest, the snapshot+deploy flow.
func (m *Manager) check(ctx context.Context, s adoptedStack, now time.Time) {
	stateful := isStateful(ctx, m.StateDir, s.Name)
	ov, err := m.Files.get(s.Name)
	if err != nil {
		m.recordCheckFailed(s.Name, err.Error())
		return
	}
	policy, _ := EffectivePolicy(m.Cfg, ov, stateful)

	// Not-running stacks have nothing to update in place.
	if len(s.Services) == 0 {
		m.storeReport(&report{stack: s.Name, state: StateNotRunning, policy: policy, stateful: stateful, checkedAt: now})
		m.markChecked(s.Name, now)
		return
	}

	var svcs []serviceReport
	anyAvailable := false
	anyUnresolvable := false
	for _, svc := range s.Services {
		local, remote, cerr := m.Digests.Check(ctx, svc.Image)
		sr := serviceReport{Service: svc.Name, Image: svc.Image, Local: local, Remote: remote, Outcome: OutcomeUpToDate}
		if cerr != nil {
			sr.Outcome = OutcomeUnresolvable
			sr.Detail = cerr.Error()
			anyUnresolvable = true
		} else if remote == "" || local == "" {
			sr.Outcome = OutcomeUnresolvable
			sr.Detail = "digest unresolvable (pinned or locally built)"
			anyUnresolvable = true
		} else if remote != local {
			sr.Outcome = OutcomeAvailable
			anyAvailable = true
		}
		svcs = append(svcs, sr)
	}

	state := StateUpToDate
	if anyAvailable {
		state = StateAvailable
	} else if anyUnresolvable && !anyAvailable {
		state = StateUnresolvable
	}

	m.storeReport(&report{stack: s.Name, state: state, policy: policy, stateful: stateful, checkedAt: now, services: svcs})
	m.markChecked(s.Name, now)

	if !anyAvailable {
		return
	}

	// Shoutrrr on every event (ADR 0009): a newer digest is
	// an event whether or not we redeploy.
	detail := availableDetail(svcs)
	m.notify(ctx, EventUpdateAvailable, s.Name, detail)
	if policy == PolicyAuto {
		m.runAuto(ctx, s.Name, detail)
	}
}

// run performs the snapshot-before-update + health-gated deploy
// flow for one stack (the `auto` path, and POST /update/run).
func (m *Manager) run(ctx context.Context, stack, reason string) *api.UpdateRun {
	now := time.Now()
	snap, err := m.Updater.Snapshot(ctx, stack)
	if err != nil {
		m.notify(ctx, EventUpdateSnapshotFail, stack, err.Error())
		m.audit("peer", "update.run", stack, 502, "snapshot failed: "+err.Error())
		return &api.UpdateRun{Stack: stack, At: now.UTC().Format(time.RFC3339), Snapshot: snap, OK: false, Detail: "snapshot failed: " + err.Error()}
	}
	healthy, detail, derr := m.Updater.Deploy(ctx, stack)
	run := &api.UpdateRun{Stack: stack, At: now.UTC().Format(time.RFC3339), Snapshot: snap, OK: healthy && derr == nil, Detail: detail}
	if derr != nil {
		run.Detail = derr.Error()
	}
	if run.OK {
		m.notify(ctx, EventUpdateOK, stack, "healthy after update ("+reason+")")
		m.audit("peer", "update.run", stack, 200, "snapshot "+snap+"; healthy")
	} else {
		m.notify(ctx, EventUpdateFailed, stack, run.Detail)
		m.audit("peer", "update.run", stack, 502, "snapshot "+snap+"; "+run.Detail)
	}
	m.mu.Lock()
	if r, ok := m.reports[stack]; ok && r != nil {
		r.mu.Lock()
		rr := *run
		r.lastRun = &rr
		r.snapshot = snap
		r.mu.Unlock()
	}
	m.mu.Unlock()
	return run
}

func (m *Manager) runAuto(ctx context.Context, stack, reason string) {
	m.run(ctx, stack, reason)
}

// availableDetail names the services with a newer digest.
func availableDetail(svcs []serviceReport) string {
	var names []string
	for _, s := range svcs {
		if s.Outcome == OutcomeAvailable {
			names = append(names, s.Service)
		}
	}
	sort.Strings(names)
	return "newer digest: " + strings.Join(names, ", ")
}

func (m *Manager) notify(ctx context.Context, ev Event, subject, detail string) {
	if m.Notifier != nil {
		m.Notifier.Notify(ctx, ev, subject, detail)
	}
}

func (m *Manager) audit(actor, action, target string, status int, detail string) {
	if m.Audit != nil {
		m.Audit.Append(actor, action, target, status, detail)
	}
}

func (m *Manager) storeReport(r *report) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if old, ok := m.reports[r.stack]; ok && old != nil {
		old.mu.Lock()
		r.lastRun = old.lastRun
		old.mu.Unlock()
	}
	m.reports[r.stack] = r
}

func (m *Manager) markChecked(stack string, at time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.last[stack] = at
}

func (m *Manager) recordCheckFailed(stack, detail string) {
	m.storeReport(&report{stack: stack, state: StateCheckFailed, checkedAt: time.Now(), err: detail})
	log.Printf("ferriage update: %s check failed: %s", stack, detail)
}

// isStateful reports whether the stack's rendered Compose config
// declares a named volume. Rendered, not parsed: the heuristic
// runs on what Compose would actually create.
func isStateful(ctx context.Context, stateDir, stack string) bool {
	// Rendered via compose config; on any failure fall back to
	// a text scan for a named volume declaration.
	if rendered, err := renderCompose(ctx, filepath.Join(stateDir, stack)); err == nil {
		return renderedHasNamedVolume(rendered)
	}
	return textHasNamedVolume(filepath.Join(stateDir, stack, "compose.yaml"))
}

// --- api conversion ---

func (r *report) toAPI() api.UpdateStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	svcs := r.services
	if svcs == nil {
		svcs = []api.UpdateService{}
	}
	out := api.UpdateStatus{
		Stack:     r.stack,
		State:     r.state,
		Policy:    r.policy,
		Stateful:  r.stateful,
		CheckedAt: r.checkedAt.UTC().Format(time.RFC3339),
		Services:  svcs,
	}
	if r.checkedAt.IsZero() {
		out.CheckedAt = ""
	}
	if r.lastRun != nil {
		lr := *r.lastRun
		out.LastRun = &lr
	}
	if r.snapshot != "" {
		out.Snapshot = r.snapshot
	}
	if r.err != "" {
		out.Error = r.err
	}
	return out
}

// States returns every known stack's report, sorted by name,
// seeding never-checked stacks from the state dir listing.
func (m *Manager) Reports() []api.UpdateStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]api.UpdateStatus, 0, len(m.reports))
	for _, r := range m.reports {
		out = append(out, r.toAPI())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Stack < out[j].Stack })
	return out
}

// Report returns one stack's report, or nil if unknown.
func (m *Manager) Report(stack string) *api.UpdateStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.reports[stack]
	if !ok {
		return nil
	}
	st := r.toAPI()
	return &st
}

// SeedFromDisk ensures every stack directory has a report entry
// (state "not-running" until its first check), so GET /update
// lists stacks the loop hasn't reached yet.
func (m *Manager) SeedFromDisk() {
	entries, err := os.ReadDir(m.StateDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() || e.Name() == identity.DirName {
			continue
		}
		m.mu.Lock()
		if _, ok := m.reports[e.Name()]; !ok {
			stateful := isStateful(context.Background(), m.StateDir, e.Name())
			ov, _ := m.Files.get(e.Name())
			policy, _ := EffectivePolicy(m.Cfg, ov, stateful)
			m.reports[e.Name()] = &report{stack: e.Name(), state: StateNotRunning, policy: policy, stateful: stateful}
		}
		m.mu.Unlock()
	}
}

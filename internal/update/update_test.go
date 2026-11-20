package update

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakePeers returns a fixed adopted-stack list.
type fakePeers struct {
	stacks []adoptedStack
	err    error
}

func (f fakePeers) List(context.Context) ([]adoptedStack, error) {
	return f.stacks, f.err
}

// fakeDigests returns per-image local/remote digests.
type fakeDigests struct {
	local  map[string]string
	remote map[string]string
	errs   map[string]error
}

func (f fakeDigests) Check(_ context.Context, image string) (string, string, error) {
	if e, ok := f.errs[image]; ok {
		return "", "", e
	}
	return f.local[image], f.remote[image], nil
}

// fakeUpdater records snapshot/deploy calls.
type fakeUpdater struct {
	snapPath string
	snapErr  error
	healthy  bool
	detail   string
	depErr   error

	snaps   int
	deploys int
}

func (f *fakeUpdater) Snapshot(context.Context, string) (string, error) {
	f.snaps++
	return f.snapPath, f.snapErr
}

func (f *fakeUpdater) Deploy(context.Context, string) (bool, string, error) {
	f.deploys++
	return f.healthy, f.detail, f.depErr
}

// fakeNotifier records events.
type fakeNotifier struct {
	events []Event
}

func (f *fakeNotifier) Notify(_ context.Context, ev Event, _, _ string) {
	f.events = append(f.events, ev)
}

// fakeAudit records machine actions.
type fakeAudit struct {
	entries []string
}

func (f *fakeAudit) Append(actor, action, target string, _ int, _ string) {
	f.entries = append(f.entries, actor+"/"+action+"/"+target)
}

func TestEffectivePolicyStatefulNeverInheritsAuto(t *testing.T) {
	cfg := Config{DefaultPolicy: PolicyAuto, DefaultInterval: 24 * time.Hour}
	p, _ := EffectivePolicy(cfg, PolicyOverride{}, false)
	if p != PolicyAuto {
		t.Errorf("stateless with global auto: got %q, want auto", p)
	}
	p, _ = EffectivePolicy(cfg, PolicyOverride{}, true)
	if p != PolicyPoll {
		t.Errorf("stateful with global auto: got %q, want poll (DBs default poll)", p)
	}
	// Explicit opt-in wins even for stateful stacks.
	p, _ = EffectivePolicy(cfg, PolicyOverride{Policy: PolicyAuto, HasPolicy: true}, true)
	if p != PolicyAuto {
		t.Errorf("explicit auto on stateful: got %q, want auto", p)
	}
}

func TestPolicyFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	b := NewFileBackend(dir)
	if _, err := b.set("demo", PolicyAuto, "6h"); err != nil {
		t.Fatalf("set: %v", err)
	}
	ov, err := b.get("demo")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !ov.HasPolicy || ov.Policy != PolicyAuto || !ov.HasInterval || ov.Interval != 6*time.Hour {
		t.Errorf("override = %+v, want auto/6h", ov)
	}
	// Another stack's entry must survive a write to demo.
	if _, err := b.set("other", PolicyPoll, ""); err != nil {
		t.Fatalf("set other: %v", err)
	}
	ov, _ = b.get("demo")
	if !ov.HasPolicy || ov.Policy != PolicyAuto {
		t.Errorf("demo policy lost after other write: %+v", ov)
	}
	if _, err := b.set("demo", PolicyPoll, ""); err != nil {
		t.Fatalf("re-set demo: %v", err)
	}
	ov, _ = b.get("demo")
	if !ov.HasPolicy || ov.Policy != PolicyPoll || !ov.HasInterval {
		t.Errorf("demo after re-set: %+v, want poll + kept 6h interval", ov)
	}
	// Invalid values are rejected.
	if _, err := b.set("demo", "sometimes", ""); err == nil {
		t.Error("invalid policy accepted")
	}
	if _, err := b.set("demo", PolicyAuto, "nope"); err == nil {
		t.Error("invalid interval accepted")
	}
	// Missing file is empty, not an error.
	b2 := NewFileBackend(t.TempDir())
	ov, err = b2.get("nope")
	if err != nil || ov.HasPolicy {
		t.Errorf("missing file: ov=%+v err=%v", ov, err)
	}
}

func TestRenderedHasNamedVolume(t *testing.T) {
	withVol := []byte(`{"services":{"app":{}},"volumes":{"data":{"name":"x"}}}`)
	if !renderedHasNamedVolume(withVol) {
		t.Error("non-empty volumes not detected")
	}
	noVol := []byte(`{"services":{"app":{}},"volumes":{}}`)
	if renderedHasNamedVolume(noVol) {
		t.Error("empty volumes detected as stateful")
	}
	if renderedHasNamedVolume([]byte(`not json`)) {
		t.Error("bad JSON detected as stateful")
	}
}

func TestTextHasNamedVolumeFallback(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "compose.yaml")
	if err := os.WriteFile(p, []byte("services:\n  app:\n    image: x\nvolumes:\n  data:\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !textHasNamedVolume(p) {
		t.Error("text fallback missed named volume")
	}
	if err := os.WriteFile(p, []byte("services:\n  app:\n    image: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if textHasNamedVolume(p) {
		t.Error("text fallback false positive")
	}
	// A service-level volumes: must not count (indented).
	if err := os.WriteFile(p, []byte("services:\n  app:\n    volumes:\n      - data:/data\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if textHasNamedVolume(p) {
		t.Error("service-level volumes: counted as top-level")
	}
}

func TestCheckPollPolicyReportsAvailableNoDeploy(t *testing.T) {
	dir := t.TempDir()
	// Stateful stack on disk (named volume) with an available update.
	if err := os.MkdirAll(filepath.Join(dir, "db"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "db", "compose.yaml"),
		[]byte("services:\n  db:\n    image: postgres:16\n    volumes:\n      - data:/var/lib/postgresql/data\nvolumes:\n  data:\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	up := &fakeUpdater{snapPath: "snapshots/db/x.tar.gz", healthy: true}
	nt := &fakeNotifier{}
	m := NewManager(dir, Config{DefaultPolicy: PolicyPoll, DefaultInterval: time.Hour},
		fakePeers{stacks: []adoptedStack{{Name: "db", Services: []adoptedService{{Name: "db", Image: "postgres:16"}}}}},
		fakeDigests{local: map[string]string{"postgres:16": "sha256:old"}, remote: map[string]string{"postgres:16": "sha256:new"}},
		up, nt, nil)
	m.check(context.Background(), adoptedStack{Name: "db", Services: []adoptedService{{Name: "db", Image: "postgres:16"}}}, time.Now())
	st := m.Report("db")
	if st == nil || st.State != StateAvailable || st.Policy != PolicyPoll || !st.Stateful {
		t.Fatalf("report = %+v, want available/poll/stateful", st)
	}
	if up.snaps != 0 || up.deploys != 0 {
		t.Errorf("poll policy deployed: snaps=%d deploys=%d", up.snaps, up.deploys)
	}
	found := false
	for _, e := range nt.events {
		if e == EventUpdateAvailable {
			found = true
		}
	}
	if !found {
		t.Errorf("update.available not notified: %v", nt.events)
	}
}

func TestCheckAutoPolicySnapshotsThenDeploys(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "web", "compose.yaml"),
		[]byte("services:\n  app:\n    image: nginx:1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	up := &fakeUpdater{snapPath: "snapshots/web/x.tar.gz", healthy: true}
	au := &fakeAudit{}
	m := NewManager(dir, Config{DefaultPolicy: PolicyAuto, DefaultInterval: time.Hour},
		fakePeers{stacks: []adoptedStack{{Name: "web", Services: []adoptedService{{Name: "app", Image: "nginx:1"}}}}},
		fakeDigests{local: map[string]string{"nginx:1": "sha256:old"}, remote: map[string]string{"nginx:1": "sha256:new"}},
		up, &fakeNotifier{}, au)
	m.check(context.Background(), adoptedStack{Name: "web", Services: []adoptedService{{Name: "app", Image: "nginx:1"}}}, time.Now())
	if up.snaps != 1 || up.deploys != 1 {
		t.Errorf("auto: snaps=%d deploys=%d, want 1/1 (snapshot BEFORE deploy)", up.snaps, up.deploys)
	}
	st := m.Report("web")
	if st.LastRun == nil || !st.LastRun.OK || st.Snapshot != "snapshots/web/x.tar.gz" {
		t.Errorf("last run = %+v snapshot=%q", st.LastRun, st.Snapshot)
	}
	if len(au.entries) == 0 || au.entries[0] != "peer/update.run/web" {
		t.Errorf("audit = %v, want peer/update.run/web", au.entries)
	}
}

func TestSnapshotFailureAbortsDeploy(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "web", "compose.yaml"),
		[]byte("services:\n  app:\n    image: nginx:1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	up := &fakeUpdater{snapErr: errBoom, healthy: true}
	m := NewManager(dir, Config{DefaultPolicy: PolicyAuto, DefaultInterval: time.Hour},
		fakePeers{}, fakeDigests{}, up, &fakeNotifier{}, nil)
	run := m.run(context.Background(), "web", "test")
	if run.OK {
		t.Error("run OK despite snapshot failure")
	}
	if up.deploys != 0 {
		t.Errorf("deployed without snapshot: deploys=%d", up.deploys)
	}
}

func TestUnresolvableNeverUpToDate(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "web", "compose.yaml"),
		[]byte("services:\n  app:\n    image: local:1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := NewManager(dir, Config{DefaultPolicy: PolicyPoll, DefaultInterval: time.Hour},
		fakePeers{}, fakeDigests{local: map[string]string{"local:1": "sha256:x"}},
		&fakeUpdater{}, nil, nil)
	m.check(context.Background(), adoptedStack{Name: "web", Services: []adoptedService{{Name: "app", Image: "local:1"}}}, time.Now())
	st := m.Report("web")
	if st == nil || st.State != StateUnresolvable {
		t.Fatalf("state = %q, want unresolvable (empty remote is not up-to-date)", st.State)
	}
}

var errBoom = errTest("boom")

type errTest string

func (e errTest) Error() string { return string(e) }

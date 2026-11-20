package update

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/NopeNix/Ferriage/internal/adopt"
	"github.com/NopeNix/Ferriage/internal/deploy"
	"github.com/NopeNix/Ferriage/internal/snapshot"
)

// prodPeers adapts adopt.List onto PeerLister.
type prodPeers struct{}

func (prodPeers) List(ctx context.Context) ([]adoptedStack, error) {
	cli, err := adopt.NewClient()
	if err != nil {
		return nil, err
	}
	defer func() { _ = cli.Close() }()
	stacks, err := adopt.List(ctx, cli)
	if err != nil {
		return nil, err
	}
	out := make([]adoptedStack, 0, len(stacks))
	for _, s := range stacks {
		a := adoptedStack{Name: s.Name}
		for _, svc := range s.Services {
			if svc.Image == "" {
				continue
			}
			a.Services = append(a.Services, adoptedService{Name: svc.Name, Image: svc.Image})
		}
		out = append(out, a)
	}
	return out, nil
}

// prodUpdater implements DeployAndSnapshot with the real snapshot
// and deploy packages.
type prodUpdater struct {
	StateDir string
	PeerID   string
}

func (p *prodUpdater) Snapshot(ctx context.Context, stack string) (string, error) {
	backend, done, err := snapshot.BackendFor()
	if err != nil {
		return "", err
	}
	defer done()
	dir := filepath.Join(p.StateDir, ".ferriage", "snapshots", stack)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("snapshot dir: %w", err)
	}
	name := time.Now().UTC().Format("20060102T150405Z") + ".tar.gz"
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("snapshot file: %w", err)
	}
	defer func() { _ = f.Close() }()
	if err := snapshot.Export(ctx, f, p.StateDir, stack, p.PeerID, backend); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return filepath.Join("snapshots", stack, name), nil
}

func (p *prodUpdater) Deploy(ctx context.Context, stack string) (bool, string, error) {
	res, err := deploy.Deploy(ctx, filepath.Join(p.StateDir, stack), deploy.DefaultConfig())
	if err != nil {
		if res != nil {
			return false, res.Detail, err
		}
		return false, "", err
	}
	if res == nil {
		return false, "no result", nil
	}
	return res.Healthy, res.Detail, nil
}

// NewProdManager builds the production Manager: real adopt,
// digest, snapshot, and deploy backends. Audit and Notifier are
// wired by the caller (cmd/peer) via SetNotifier/SetAudit so
// this package never imports the audit/notify implementations.
func NewProdManager(stateDir string, cfg Config, peerID string) (*Manager, error) {
	checker, err := NewDockerChecker()
	if err != nil {
		return nil, err
	}
	m := NewManager(stateDir, cfg, prodPeers{}, checker, &prodUpdater{StateDir: stateDir, PeerID: peerID}, nil, nil)
	return m, nil
}

// SetNotifier wires the notifier (nil-safe).
func (m *Manager) SetNotifier(n Notifier) { m.Notifier = n }

// SetAudit wires the audit sink (nil-safe).
func (m *Manager) SetAudit(a AuditSink) { m.Audit = a }

// parseDur is a tiny wrapper so handler.go can share the
// duration parse without exporting time.
func parseDur(s string) (time.Duration, error) { return time.ParseDuration(s) }

func readDirNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

func nowUTC() time.Time { return time.Now() }

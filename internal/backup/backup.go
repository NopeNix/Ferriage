// Package backup implements restic-based incremental backups
// (CONCEPT: "Backup / Restore / Export (first-class)"; ADR 0011).
//
// Restic is the backup engine — never build one — and Ferriage
// shells out to the restic CLI exactly the way deploy shells out
// to `docker compose` (session 009 precedent). The repository
// lives at "<stateDir>/.ferriage/restic-repo" — never synced, the
// same rule as identity and the audit log. The repo password
// never crosses the API in either direction (ADR 0007
// discipline): FERRIAGE_RESTIC_PASSWORD or
// FERRIAGE_RESTIC_PASSWORD_FILE, read by this package.
//
// What gets backed up is the ADR 0002 payload — the stack dir
// plus captured volume contents — but incremental and
// deduplicated in the repo instead of a full tarball per run.
// Volumes are captured via the daemon's copy APIs (the
// internal/snapshot helper-container mechanism) into a temp tree;
// the temp tree is what restic actually backs up. A restore
// re-populates fresh volumes the same way and is only done when
// the stack is healthy (health-gated, like a deploy).
package backup

import (
	"archive/tar"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/docker/docker/api/types/volume"
)

// Pseudo-paths the repo tags select by.
const (
	// ScopeStack backs up one stack.
	ScopeStack = "stack"
	// ScopeNode backs up the whole node (all stacks + their
	// volumes) — the disaster-recovery unit.
	ScopeNode = "node"
)

// Snapshot is one restic snapshot in the repo. The JSON shape
// is restic's `snapshots --json` output: "summary" is an object
// with backup stats (not a bool) and "size" is absent on older
// restic — over-tolerant parsing matters, since the peer image
// pins whatever Alpine ships.
type Snapshot struct {
	ID      string          `json:"id"`
	ShortID string          `json:"short_id,omitempty"`
	Time    string          `json:"time"`
	Tags    []string        `json:"tags,omitempty"`
	Paths   []string        `json:"paths,omitempty"`
	Size    uint64          `json:"size,omitempty"`
	Summary json.RawMessage `json:"summary,omitempty"`
}

// Status is one backup surface's answer (GET /backup body).
type Status struct {
	Repo         string `json:"repo"`
	Initialized  bool   `json:"initialized"`
	Snapshots    int    `json:"snapshots"`
	PasswordSet  bool   `json:"password_set"`
	Restic       string `json:"restic,omitempty"`
	LastBackupAt string `json:"last_backup_at,omitempty"`
	LastBackupOK bool   `json:"last_backup_ok,omitempty"`
	LastError    string `json:"last_error,omitempty"`
}

// List is the body of GET /backup (newest first).
type List struct {
	Status    Status     `json:"status"`
	Snapshots []Snapshot `json:"snapshots"`
}

// Result is one run/restore/forget outcome.
type Result struct {
	Scope    string `json:"scope"`
	Stack    string `json:"stack,omitempty"`
	Snapshot string `json:"snapshot,omitempty"`
	Healthy  bool   `json:"healthy,omitempty"`
	Detail   string `json:"detail,omitempty"`
	Pruned   int    `json:"pruned,omitempty"`
}

// Error classifies a backup failure for HTTP mapping.
type Error struct {
	Code string
	Msg  string
}

func (e *Error) Error() string { return e.Msg }

func badRequest(msg string) error { return &Error{Code: "bad_request", Msg: msg} }
func notFound(msg string) error   { return &Error{Code: "not_found", Msg: msg} }
func upstream(msg string) error   { return &Error{Code: "upstream", Msg: msg} }

// password reads the repo password from its env/file sources.
// Empty when neither is configured (restic will fail closed).
func password() string {
	if v := strings.TrimSpace(os.Getenv("FERRIAGE_RESTIC_PASSWORD")); v != "" {
		return v
	}
	if f := os.Getenv("FERRIAGE_RESTIC_PASSWORD_FILE"); f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			return ""
		}
		if s := strings.TrimSpace(string(b)); s != "" {
			return s
		}
	}
	_ = os.Setenv("RESTIC_PASSWORD", "") // never leak a stale one
	return ""
}

// resticBin is the restic CLI path (FERRIAGE_RESTIC, default
// "restic").
func resticBin() string {
	if v := strings.TrimSpace(os.Getenv("FERRIAGE_RESTIC")); v != "" {
		return v
	}
	return "restic"
}

// repoDir is the repository path for a state dir.
func repoDir(stateDir string) string {
	return filepath.Join(stateDir, ".ferriage", "restic-repo")
}

// Manager runs backups against one state dir.
type Manager struct {
	StateDir string
	Keep     int    // retention: keep last N (default 7)
	Restic   string // binary override (tests); empty = env/default
	Password string // override (tests); empty = env/file
	Now      func() time.Time
}

// NewManager builds a Manager for a state dir.
func NewManager(stateDir string) *Manager {
	keep := 7
	if v := strings.TrimSpace(os.Getenv("FERRIAGE_BACKUP_KEEP")); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			keep = n
		}
	}
	return &Manager{StateDir: stateDir, Keep: keep, Now: time.Now}
}

func (m *Manager) bin() string {
	if m.Restic != "" {
		return m.Restic
	}
	return resticBin()
}

func (m *Manager) pw() string {
	if m.Password != "" {
		return m.Password
	}
	return password()
}

func (m *Manager) repo() string { return repoDir(m.StateDir) }

// volumeBackend abstracts the daemon copy surface (fakeable in
// tests); the production one is internal/snapshot's Backend.
type volumeBackend interface {
	VolumeList(ctx context.Context, options volume.ListOptions) (volume.ListResponse, error)
	VolumeInspect(ctx context.Context, volumeID string) (volume.Volume, error)
	VolumeCreate(ctx context.Context, options volume.CreateOptions) (volume.Volume, error)
	VolumeRemove(ctx context.Context, volumeID string, force bool) error
	VolumeCopyOut(ctx context.Context, volumeName string, w io.Writer) error
	VolumeCopyIn(ctx context.Context, volumeName string, r io.Reader) error
}

// BackendFunc builds the production backend for a request.
type BackendFunc func(ctx context.Context) (volumeBackend, func(), error)

// captureTree stages the ADR 0002 payload of one stack into dir:
// stack files under dir/stack, volume contents under
// dir/volumes/<short-name>/, using the same daemon copy capture
// as internal/snapshot.
func captureTree(ctx context.Context, stateDir, stack string, dir string, vols volumeBackend) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("backup: stage dir: %w", err)
	}
	// Stack dir: symlink-skipping copy (ADR 0001 discipline).
	src := filepath.Join(stateDir, stack)
	if st, err := os.Stat(src); err != nil || !st.IsDir() {
		return notFound("stack not found")
	}
	if err := copyDir(src, filepath.Join(dir, "stack")); err != nil {
		return fmt.Errorf("backup: stage stack dir: %w", err)
	}
	// Volumes: resolve the same way snapshot does (project label
	// or <stack>_<name> prefix) and capture via the copy API.
	resp, err := vols.VolumeList(ctx, volume.ListOptions{})
	if err != nil {
		return fmt.Errorf("backup: list volumes: %w", err)
	}
	for _, v := range resp.Volumes {
		if v.Labels["com.docker.compose.project"] != stack && !strings.HasPrefix(v.Name, stack+"_") {
			continue
		}
		name := v.Name
		if rest, ok := strings.CutPrefix(name, stack+"_"); ok && rest != "" {
			name = rest
		}
		if name == "" || strings.ContainsAny(name, "/\\") || name == "." || name == ".." {
			return fmt.Errorf("backup: volume name %q is not archive-safe", v.Name)
		}
		dest := filepath.Join(dir, "volumes", name)
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return fmt.Errorf("backup: stage volume dir: %w", err)
		}
		if err := untarVolume(ctx, vols, v.Name, dest); err != nil {
			return fmt.Errorf("backup: capture volume %s: %w", name, err)
		}
	}
	return nil
}

func untarVolume(ctx context.Context, vols volumeBackend, full, dest string) error {
	raw, err := volsCopyOutBytes(ctx, vols, full)
	if err != nil {
		return err
	}
	tr := tar.NewReader(strings.NewReader(string(raw)))
	root := ""
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read volume archive: %w", err)
		}
		name := strings.TrimPrefix(filepath.ToSlash(hdr.Name), "./")
		if name == "" || name == "." {
			continue
		}
		if root == "" {
			if i := strings.Index(name, "/"); i >= 0 {
				root = name[:i]
			}
		}
		if root != "" && strings.HasPrefix(name, root+"/") {
			name = strings.TrimPrefix(name, root+"/")
		}
		if err := safeRel(name); err != nil {
			return err
		}
		destPath := filepath.Join(dest, filepath.FromSlash(name))
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return fmt.Errorf("create dir: %w", err)
		}
		f, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return fmt.Errorf("create file: %w", err)
		}
		if _, err := io.Copy(f, tr); err != nil {
			_ = f.Close()
			return fmt.Errorf("write file: %w", err)
		}
		_ = f.Close()
	}
	return nil
}

func volsCopyOutBytes(ctx context.Context, vols volumeBackend, full string) ([]byte, error) {
	var buf bytesBuffer
	if err := vols.VolumeCopyOut(ctx, full, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type bytesBuffer struct{ b []byte }

func (b *bytesBuffer) Write(p []byte) (int, error) { b.b = append(b.b, p...); return len(p), nil }
func (b *bytesBuffer) Bytes() []byte               { return b.b }

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		dest := filepath.Join(dst, rel)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = in.Close() }()
		out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return err
		}
		_, cerr := io.Copy(out, in)
		if cerr != nil {
			_ = out.Close()
			return cerr
		}
		return out.Close()
	})
}

// safeRel rejects archive paths that could escape a dir.
func safeRel(rel string) error {
	if rel == "" || strings.HasPrefix(rel, "/") || strings.Contains(rel, "\\") {
		return fmt.Errorf("invalid archive path: %s", rel)
	}
	clean := filepath.ToSlash(filepath.Clean(rel))
	if clean != rel || clean == "." {
		return fmt.Errorf("invalid archive path: %s", rel)
	}
	return nil
}

// --- restic invocation ---

func (m *Manager) run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, m.bin(), args...)
	cmd.Env = append(os.Environ(), "RESTIC_REPOSITORY="+m.repo(), "RESTIC_PASSWORD="+m.pw())
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", upstream(fmt.Sprintf("restic %s: %v: %s", strings.Join(args, " "), err, truncate(errb.String(), 300)))
	}
	return out.String(), nil
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		s = s[:n] + "..."
	}
	return s
}

// ensureRepo creates the repo if missing (init on first use).
func (m *Manager) ensureRepo(ctx context.Context) error {
	if st, err := os.Stat(m.repo()); err == nil && st.IsDir() {
		return nil
	}
	if m.pw() == "" {
		return upstream("restic password not configured (set FERRIAGE_RESTIC_PASSWORD or FERRIAGE_RESTIC_PASSWORD_FILE on the peer)")
	}
	if err := os.MkdirAll(m.repo(), 0o755); err != nil {
		return fmt.Errorf("backup: create repo dir: %w", err)
	}
	if _, err := m.run(ctx, "init"); err != nil {
		return err
	}
	return nil
}

// Run backs up one stack (or the whole node when stack == "").
func (m *Manager) Run(ctx context.Context, stack string, vols volumeBackend) (*Result, error) {
	scope := ScopeNode
	target := ""
	if stack != "" {
		scope = ScopeStack
		if _, err := os.Stat(filepath.Join(m.StateDir, stack)); err != nil {
			return nil, notFound("stack not found")
		}
		target = stack
	}
	pw := m.pw()
	if pw == "" {
		return nil, upstream("restic password not configured (set FERRIAGE_RESTIC_PASSWORD or FERRIAGE_RESTIC_PASSWORD_FILE on the peer)")
	}
	if err := m.ensureRepo(ctx); err != nil {
		return nil, err
	}
	// Stage the payload into a temp tree; restic backs up the
	// tree. A failed stage never leaves a half snapshot.
	tmp, err := os.MkdirTemp("", "ferriage-backup-*")
	if err != nil {
		return nil, fmt.Errorf("backup: stage: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if stack != "" {
		if err := captureTree(ctx, m.StateDir, stack, tmp, vols); err != nil {
			return nil, err
		}
	} else {
		// Node: every stack dir under the state dir (never
		// .ferriage/), each staged its own subtree.
		entries, err := os.ReadDir(m.StateDir)
		if err != nil {
			return nil, fmt.Errorf("backup: read state dir: %w", err)
		}
		names := []string{}
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				names = append(names, e.Name())
			}
		}
		sort.Strings(names)
		for _, name := range names {
			sub := filepath.Join(tmp, name)
			if err := captureTree(ctx, m.StateDir, name, sub, vols); err != nil {
				return nil, err
			}
		}
		target = "node"
	}
	tags := []string{"ferriage", scope}
	if stack != "" {
		tags = append(tags, "stack="+stack)
	}
	args := []string{"backup", "--tag", strings.Join(tags, ","), "--host", "ferriage", tmp}
	out, err := m.run(ctx, args...)
	if err != nil {
		return nil, err
	}
	// Apply retention (prunes old snapshots beyond keep).
	_, _ = m.run(ctx, "forget", "--keep-last", fmt.Sprint(m.Keep), "--prune")
	_ = out
	snapID := parseSnapshotID(out)
	return &Result{Scope: scope, Stack: target, Snapshot: snapID, Detail: "backup complete"}, nil
}

func parseSnapshotID(out string) string {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		// restic 0.16 "snapshot <id> saved" or 0.17+ similar.
		if strings.HasPrefix(line, "snapshot ") && strings.HasSuffix(line, " saved") {
			return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "snapshot "), " saved"))
		}
	}
	if strings.Contains(out, "unable to") {
		return ""
	}
	return ""
}

// Restore picks a snapshot (id or empty for latest) and restores
// it, then deploys health-gated when deploy is true.
func (m *Manager) Restore(ctx context.Context, stack, snapID string, vols volumeBackend, deployFn func(ctx context.Context, stack string) (bool, string, error)) (*Result, error) {
	if m.pw() == "" {
		return nil, upstream("restic password not configured")
	}
	if err := m.ensureRepo(ctx); err != nil {
		return nil, err
	}
	// Find the snapshot: latest of this stack if id empty.
	target := ""
	if snapID == "" {
		snaps, err := m.List(ctx)
		if err != nil {
			return nil, err
		}
		if len(snaps.Snapshots) == 0 {
			return nil, notFound("no snapshots")
		}
		snapID = snaps.Snapshots[0].ID
		target = snapID
	}
	// Restore into a temp tree, then install.
	tmp, err := os.MkdirTemp("", "ferriage-restore-*")
	if err != nil {
		return nil, fmt.Errorf("backup: restore stage: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if _, err := m.run(ctx, "restore", snapID, "--target", tmp); err != nil {
		return nil, err
	}
	// The restore target holds the staged payload: for a stack
	// backup, <tmp>/<something>/stack; for node, per-stack dirs.
	// Detect: if tmp has a "stack" entry at its root, it's a
	// single-stack payload; else every subdir is a stack.
	nodeMode := false
	var stacks []string
	// restic records the staged payload by its absolute tmp
	// path; after restore it nests under <target>/tmp/... So
	// always walk for the payload rather than checking the
	// restore root.
	if found := findStackPayload(tmp); found != "" && stack != "" {
		// single-stack payload (and the caller named it)
		if err := installStack(ctx, m.StateDir, stack, found, vols); err != nil {
			return nil, err
		}
		stacks = []string{stack}
	} else {
		nodeMode = true
		// Node restore: each subdir under the restored tree…
		roots, _ := findNodePayloads(tmp)
		if len(roots) == 0 {
			return nil, upstream("restored snapshot has no stack payload")
		}
		for _, r := range roots {
			name := filepath.Base(r)
			if stack != "" && name != stack {
				continue
			}
			if err := installStack(ctx, m.StateDir, name, r, vols); err != nil {
				return nil, err
			}
			stacks = append(stacks, name)
		}
	}
	_ = nodeMode
	if len(stacks) == 0 {
		return nil, notFound("no stacks to restore")
	}
	healthy := false
	detail := "restored"
	if deployFn != nil {
		h, d, derr := deployFn(ctx, stack)
		if derr != nil {
			return &Result{Scope: "restore", Stack: stack, Snapshot: target, Healthy: false, Detail: fmt.Sprintf("restored but deploy failed: %v (%s)", derr, d)}, nil
		}
		healthy = h
		detail = d
	}
	return &Result{Scope: "restore", Stack: stack, Snapshot: target, Healthy: healthy, Detail: detail}, nil
}

func findStackPayload(root string) string {
	// restic records the staged tree by its absolute (tmpdir)
	// path: <tmp>/stack, <tmp>/volumes. After `restore
	// --target`, those land at <target>/tmp/<tmpdir>/... — the
	// "ferriage-backup-*" dir is the payload root. Walk for any
	// dir named "ferriage-backup-*" holding a "stack" subdir;
	// fall back to the first "stack" dir's parent.
	var byTmp string
	var byStack string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if strings.HasPrefix(d.Name(), "ferriage-backup-") {
			if st, _ := os.Stat(filepath.Join(path, "stack")); st != nil && st.IsDir() {
				byTmp = path
				return filepath.SkipDir
			}
		}
		if d.Name() == "stack" && byStack == "" {
			byStack = filepath.Dir(path)
		}
		return nil
	})
	if byTmp != "" {
		return byTmp
	}
	return byStack
}

func findNodePayloads(root string) ([]string, string) {
	// Node backups stage each stack under <tmp>/<stackname>/.
	// After restore, each appears at some depth under root.
	// Every dir named "ferriage-backup-*" that holds one or
	// more "<stackname>/stack" children is a payload parent.
	roots := []string{}
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() || !strings.HasPrefix(d.Name(), "ferriage-backup-") {
			return nil
		}
		entries, _ := os.ReadDir(path)
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			if st, _ := os.Stat(filepath.Join(path, e.Name(), "stack")); st != nil && st.IsDir() {
				roots = append(roots, filepath.Join(path, e.Name()))
			}
		}
		return filepath.SkipDir
	})
	sort.Strings(roots)
	return roots, ""
}

// installStack copies a restored payload root (holding "stack"
// and "volumes") into the state dir and fresh volumes. Refuses an
// existing stack dir (ADR 0002 discipline).
func installStack(ctx context.Context, stateDir, stack, payloadRoot string, vols volumeBackend) error {
	stackDir := filepath.Join(stateDir, stack)
	if _, err := os.Stat(stackDir); err == nil {
		return badRequest("stack already exists")
	}
	srcStack := filepath.Join(payloadRoot, "stack")
	if err := os.MkdirAll(filepath.Join(stateDir, stack), 0o755); err != nil {
		return fmt.Errorf("backup: create stack dir: %w", err)
	}
	if err := copyDir(srcStack, stackDir); err != nil {
		return fmt.Errorf("backup: install stack files: %w", err)
	}
	// Volumes: create fresh and populate.
	volDir := filepath.Join(payloadRoot, "volumes")
	entries, _ := os.ReadDir(volDir)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		full := stack + "_" + name
		if _, err := vols.VolumeInspect(ctx, full); err == nil {
			return badRequest("volume already exists: " + full)
		}
		if _, err := vols.VolumeCreate(ctx, volume.CreateOptions{Name: full}); err != nil {
			return fmt.Errorf("backup: create volume %s: %w", full, err)
		}
		if err := populateVolume(ctx, vols, filepath.Join(volDir, name), full); err != nil {
			return fmt.Errorf("backup: populate volume %s: %w", name, err)
		}
	}
	return nil
}

func populateVolume(ctx context.Context, vols volumeBackend, src, full string) error {
	tf, err := os.CreateTemp("", "ferriage-vol-restore-*.tar")
	if err != nil {
		return fmt.Errorf("backup: temp volume archive: %w", err)
	}
	defer func() { _ = os.Remove(tf.Name()) }()
	tw := tar.NewWriter(tf)
	err = filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = in.Close() }()
		hdr := &tar.Header{Name: filepath.ToSlash(rel), Mode: 0o644, Size: 0}
		st, _ := in.Stat()
		hdr.Size = st.Size()
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		_, err = io.Copy(tw, in)
		return err
	})
	if err != nil {
		_ = tw.Close()
		_ = tf.Close()
		return fmt.Errorf("backup: tar volume dir: %w", err)
	}
	if err := tw.Close(); err != nil {
		_ = tf.Close()
		return fmt.Errorf("backup: tar volume dir: %w", err)
	}
	if err := tf.Close(); err != nil {
		return fmt.Errorf("backup: tar volume dir: %w", err)
	}
	in, err := os.Open(tf.Name())
	if err != nil {
		return fmt.Errorf("backup: open volume archive: %w", err)
	}
	defer func() { _ = in.Close() }()
	if err := vols.VolumeCopyIn(ctx, full, in); err != nil {
		return fmt.Errorf("backup: copy into volume %s: %w", full, err)
	}
	return nil
}

// Forget applies retention pruning.
func (m *Manager) Forget(ctx context.Context) (*Result, error) {
	if m.pw() == "" {
		return nil, upstream("restic password not configured")
	}
	if _, err := m.run(ctx, "forget", "--keep-last", fmt.Sprint(m.Keep), "--prune"); err != nil {
		return nil, err
	}
	return &Result{Scope: "forget", Detail: fmt.Sprintf("kept last %d", m.Keep)}, nil
}

// List reads repo status and snapshot history (newest first).
func (m *Manager) List(ctx context.Context) (*List, error) {
	st := Status{Repo: m.repo(), PasswordSet: m.pw() != "", Restic: m.bin()}
	initialized := false
	if _, err := os.Stat(m.repo()); err == nil {
		initialized = true
	}
	st.Initialized = initialized
	if !initialized || m.pw() == "" {
		return &List{Status: st, Snapshots: []Snapshot{}}, nil
	}
	out, err := m.run(ctx, "snapshots", "--json")
	if err != nil {
		return nil, err
	}
	snaps := []Snapshot{}
	if strings.TrimSpace(out) != "" {
		if err := json.Unmarshal([]byte(out), &snaps); err != nil {
			return nil, upstream(fmt.Sprintf("restic snapshots --json: %v", err))
		}
	}
	sort.Slice(snaps, func(i, j int) bool { return snaps[i].Time > snaps[j].Time })
	st.Snapshots = len(snaps)
	if len(snaps) > 0 {
		st.LastBackupAt = snaps[0].Time
		st.LastBackupOK = true
	}
	return &List{Status: st, Snapshots: snaps}, nil
}

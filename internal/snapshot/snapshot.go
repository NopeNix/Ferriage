// Package snapshot implements the Phase 1.4 snapshot format of ADR
// 0002: one gzip+tar file containing manifest.json, the stack's files,
// and the contents of its named volumes. Written and read with the Go
// standard library only; streams in both directions.
package snapshot

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/docker/docker/api/types/volume"
)

// FormatVersion is the snapshot format version this package writes.
const FormatVersion = 1

const manifestName = "manifest.json"

// Manifest is the archive's manifest.json entry.
type Manifest struct {
	FormatVersion int       `json:"format_version"`
	Stack         string    `json:"stack"`
	CreatedAt     time.Time `json:"created_at"`
	CreatedBy     string    `json:"created_by,omitempty"`
	Files         []string  `json:"files"`
	Volumes       []Voldesc `json:"volumes,omitempty"`
}

// Voldesc describes one captured volume.
type Voldesc struct {
	Name string `json:"name"`
}

// Error is a snapshot failure (HTTP-mapped by the handler).
type Error struct {
	Code string
	Msg  string
}

func (e *Error) Error() string { return e.Msg }

func badRequest(msg string) error { return &Error{Code: "bad_request", Msg: msg} }
func notFound(msg string) error   { return &Error{Code: "not_found", Msg: msg} }

// Backend reads and creates volumes on the daemon. An interface so
// tests can fake it; the Docker client implements it.
type Backend interface {
	VolumeList(ctx context.Context, options volume.ListOptions) (volume.ListResponse, error)
	VolumeInspect(ctx context.Context, volumeID string) (volume.Volume, error)
	VolumeCreate(ctx context.Context, options volume.CreateOptions) (volume.Volume, error)
	VolumeRemove(ctx context.Context, volumeID string, force bool) error
	// VolumeCopyOut copies the contents of volumeName's mountpoint to
	// w as a tar archive; VolumeCopyIn extracts the tar read from r
	// into the volume. Both are the daemon's first-class copy APIs,
	// not a helper shelling out to tar.
	VolumeCopyOut(ctx context.Context, volumeName string, w io.Writer) error
	VolumeCopyIn(ctx context.Context, volumeName string, r io.Reader) error
}

// volumeRef is the resolved export source of one named volume.
type volumeRef struct {
	name string
	full string // full daemon volume name (<project>_<name>)
}

// resolveVolumes lists every named volume belonging to the stack:
// accepted when either the compose project label matches or the name
// carries the compose `<project>_<name>` prefix (restore-created
// volumes have the prefix but no labels). A stack with no volumes
// yields an empty list.
func resolveVolumes(ctx context.Context, c Backend, stack string) ([]volumeRef, error) {
	resp, err := c.VolumeList(ctx, volume.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("snapshot: list volumes: %w", err)
	}
	var out []volumeRef
	for _, v := range resp.Volumes {
		if v.Labels["com.docker.compose.project"] != stack &&
			!strings.HasPrefix(v.Name, stack+"_") {
			continue
		}
		name := v.Name
		// Compose names volumes <project>_<name>; the archive
		// stores them by their short name.
		if rest, ok := strings.CutPrefix(name, stack+"_"); ok && rest != "" {
			name = rest
		}
		if name == "" || strings.ContainsAny(name, "/\\") || name == "." || name == ".." {
			return nil, fmt.Errorf("snapshot: volume name %q is not archive-safe", v.Name)
		}
		out = append(out, volumeRef{name: name, full: v.Name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out, nil
}

// walkFiles lists regular files under dir, as slash paths relative to
// dir, sorted. Symlinks are skipped (ADR 0001 discipline).
func walkFiles(dir string) ([]string, error) {
	var rels []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rels = append(rels, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(rels)
	return rels, nil
}

type archiveEntry struct {
	arc string // archive path
	src string // host file to read
}

// plan returns the stack-file entries for one export. Volume entries
// are streamed separately (see streamVolume); the manifest lists both.
func planStack(stackDir string) ([]archiveEntry, error) {
	files, err := walkFiles(stackDir)
	if err != nil {
		return nil, fmt.Errorf("snapshot: walk stack dir: %w", err)
	}
	var entries []archiveEntry
	for _, rel := range files {
		entries = append(entries, archiveEntry{"stack/" + rel, filepath.Join(stackDir, filepath.FromSlash(rel))})
	}
	return entries, nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Export writes the snapshot archive for stack (found under stateDir)
// to w. Volume contents are captured via helper containers (vols).
func Export(ctx context.Context, w io.Writer, stateDir, stack, peerID string, vols Backend) error {
	stackDir := filepath.Join(stateDir, stack)
	if st, err := os.Stat(stackDir); err != nil || !st.IsDir() {
		return notFound("stack not found")
	}
	volRefs, err := resolveVolumes(ctx, vols, stack)
	if err != nil {
		return err
	}
	stackEntries, err := planStack(stackDir)
	if err != nil {
		return err
	}

	// Capture each volume into a temp tar, then walk that tar to
	// build the manifest entry list. Volumes are small at milestone
	// scale; the temp tar keeps export streaming past the manifest.
	var entryNames []string
	for _, e := range stackEntries {
		entryNames = append(entryNames, e.arc)
	}
	type captured struct {
		ref     volumeRef
		tarPath string
		files   []string
	}
	var caps []*captured
	for _, v := range volRefs {
		tf, err := os.CreateTemp("", "ferriage-vol-*.tar")
		if err != nil {
			return fmt.Errorf("snapshot: temp volume archive: %w", err)
		}
		if err := vols.VolumeCopyOut(ctx, v.full, tf); err != nil {
			_ = tf.Close()
			_ = os.Remove(tf.Name())
			return fmt.Errorf("snapshot: capture volume %s: %w", v.name, err)
		}
		if cerr := tf.Close(); cerr != nil {
			_ = os.Remove(tf.Name())
			return fmt.Errorf("snapshot: capture volume %s: %w", v.name, cerr)
		}
		files, err := listTar(tf.Name())
		if err != nil {
			_ = os.Remove(tf.Name())
			return fmt.Errorf("snapshot: inspect volume %s archive: %w", v.name, err)
		}
		for _, f := range files {
			entryNames = append(entryNames, "volumes/"+v.name+"/"+f)
		}
		caps = append(caps, &captured{ref: v, tarPath: tf.Name(), files: files})
	}
	defer func() {
		for _, c := range caps {
			_ = os.Remove(c.tarPath)
		}
	}()

	names := make([]string, 0, len(entryNames))
	names = append(names, entryNames...)
	sort.Strings(names)
	desc := make([]Voldesc, 0, len(volRefs))
	for _, v := range volRefs {
		desc = append(desc, Voldesc{Name: v.name})
	}
	manifest := Manifest{
		FormatVersion: FormatVersion,
		Stack:         stack,
		CreatedAt:     time.Now().UTC(),
		CreatedBy:     peerID,
		Files:         names,
		Volumes:       desc,
	}
	mb, err := json.Marshal(manifest)
	if err != nil {
		return err
	}

	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	if err := writeEntry(tw, manifestName, mb); err != nil {
		return err
	}
	for _, e := range stackEntries {
		sum, err := sha256File(e.src)
		if err != nil {
			return fmt.Errorf("snapshot: read %s: %w", e.arc, err)
		}
		if err := writeFileEntry(tw, e.arc, e.src, sum); err != nil {
			return err
		}
	}
	for _, c := range caps {
		if err := streamVolume(tw, c.tarPath, c.ref.name); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

// listTar returns the regular-file paths inside a tar archive,
// relative to its root, sorted.
func listTar(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	tw := tar.NewReader(f)
	var out []string
	for {
		hdr, err := tw.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		name := strings.TrimPrefix(filepath.ToSlash(hdr.Name), "./")
		if name == "" {
			continue
		}
		if err := safeRel(name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}

// streamVolume re-emits a volume's temp tar into the export archive
// under volumes/<name>/, hashing each file while streaming.
func streamVolume(tw *tar.Writer, tarPath, volName string) error {
	f, err := os.Open(tarPath)
	if err != nil {
		return fmt.Errorf("snapshot: open volume archive: %w", err)
	}
	defer func() { _ = f.Close() }()
	tr := tar.NewReader(f)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("snapshot: read volume archive: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		name := strings.TrimPrefix(filepath.ToSlash(hdr.Name), "./")
		if name == "" {
			continue
		}
		if err := safeRel(name); err != nil {
			return err
		}
		arc := "volumes/" + volName + "/" + name
		body, err := io.ReadAll(tr)
		if err != nil {
			return fmt.Errorf("snapshot: read volume file: %w", err)
		}
		sum := sha256.Sum256(body)
		nh := &tar.Header{
			Name: arc,
			Mode: hdr.Mode,
			Size: int64(len(body)),
			PAXRecords: map[string]string{
				"ferriage.sha256": hex.EncodeToString(sum[:]),
			},
		}
		if err := tw.WriteHeader(nh); err != nil {
			return fmt.Errorf("snapshot: write %s: %w", arc, err)
		}
		if _, err := tw.Write(body); err != nil {
			return fmt.Errorf("snapshot: write %s: %w", arc, err)
		}
	}
	return nil
}

// helperImage is the one-shot helper base. Busybox tar is in every
// alpine; no pull needed when the stack already runs it (the common
// case) — with pull it is a tiny download.
const helperImage = "alpine:3.20"

func writeEntry(tw *tar.Writer, name string, data []byte) error {
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(data))}); err != nil {
		return fmt.Errorf("snapshot: write %s: %w", name, err)
	}
	if _, err := tw.Write(data); err != nil {
		return fmt.Errorf("snapshot: write %s: %w", name, err)
	}
	return nil
}

func writeFileEntry(tw *tar.Writer, arc, src, sum string) error {
	st, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("snapshot: stat %s: %w", arc, err)
	}
	mode := int64(st.Mode().Perm())
	if mode == 0 {
		mode = 0o644
	}
	hdr := &tar.Header{
		Name: arc,
		Mode: mode,
		Size: st.Size(),
		PAXRecords: map[string]string{
			"ferriage.sha256": sum,
		},
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("snapshot: write %s: %w", arc, err)
	}
	f, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("snapshot: open %s: %w", arc, err)
	}
	defer func() { _ = f.Close() }()
	if _, err := io.Copy(tw, f); err != nil {
		return fmt.Errorf("snapshot: copy %s: %w", arc, err)
	}
	return nil
}

// Restore reads a snapshot archive from r and extracts it into
// stateDir/stack and fresh volume mountpoints. It refuses to
// overwrite an existing stack directory or an existing volume
// (ADR 0002). The archive is buffered once so each entry can be
// checksum-verified while extracting; on any failure after volumes
// are created, those volumes are removed again.
func Restore(ctx context.Context, r io.Reader, stateDir, stack string, vols Backend) (*Manifest, error) {
	stackDir := filepath.Join(stateDir, stack)
	if _, err := os.Stat(stackDir); err == nil {
		return nil, badRequest("stack already exists")
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return nil, fmt.Errorf("snapshot: create state dir: %w", err)
	}
	tmpDir, err := os.MkdirTemp(stateDir, ".restore-*")
	if err != nil {
		return nil, fmt.Errorf("snapshot: create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	raw, err := io.ReadAll(io.LimitReader(r, 1<<31))
	if err != nil {
		return nil, badRequest("unreadable snapshot")
	}
	gz, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, badRequest("not a gzip snapshot")
	}
	tw := tar.NewReader(gz)

	var manifest *Manifest
	declared := map[string]struct{}{}
	seen := map[string]struct{}{}

	for {
		hdr, err := tw.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, badRequest("corrupt archive")
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		name := hdr.Name
		if err := safeArcPath(name); err != nil {
			return nil, err
		}
		if name == manifestName {
			if manifest != nil {
				return nil, badRequest("duplicate manifest")
			}
			var m Manifest
			if err := json.NewDecoder(io.LimitReader(tw, 1<<20)).Decode(&m); err != nil {
				return nil, badRequest("bad manifest")
			}
			if m.FormatVersion != FormatVersion {
				return nil, badRequest("unsupported snapshot format version")
			}
			manifest = &m
			for _, f := range manifest.Files {
				if err := safeArcPath(f); err != nil {
					return nil, err
				}
				declared[f] = struct{}{}
			}
			continue
		}
		if manifest == nil {
			return nil, badRequest("archive entry before manifest")
		}
		if _, ok := declared[name]; !ok {
			return nil, badRequest("archive entry not in manifest: " + name)
		}
		if _, dup := seen[name]; dup {
			return nil, badRequest("duplicate archive entry: " + name)
		}
		seen[name] = struct{}{}
		dest, err := entryDest(tmpDir, name)
		if err != nil {
			return nil, err
		}
		if err := extractEntry(tw, hdr, dest); err != nil {
			return nil, err
		}
	}
	if manifest == nil {
		return nil, badRequest("archive has no manifest")
	}
	if len(seen) != len(declared) {
		return nil, badRequest("archive is missing declared files")
	}

	// Create each volume fresh and populate it. Volumes created so
	// far are removed again if a later step fails.
	var created []string
	cleanup := func() {
		for _, name := range created {
			_ = vols.VolumeRemove(ctx, name, true)
		}
	}
	for _, vd := range manifest.Volumes {
		full := stack + "_" + vd.Name
		if _, err := vols.VolumeInspect(ctx, full); err == nil {
			cleanup()
			return nil, badRequest("volume already exists: " + full)
		}
		if _, err := vols.VolumeCreate(ctx, volume.CreateOptions{Name: full}); err != nil {
			cleanup()
			return nil, fmt.Errorf("snapshot: create volume %s: %w", full, err)
		}
		created = append(created, full)
		if err := populateVolume(ctx, vols, filepath.Join(tmpDir, "volumes", vd.Name), full); err != nil {
			cleanup()
			return nil, fmt.Errorf("snapshot: populate volume %s: %w", vd.Name, err)
		}
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "stack"), 0o755); err != nil {
		cleanup()
		return nil, fmt.Errorf("snapshot: create stack dir: %w", err)
	}
	if err := os.Rename(filepath.Join(tmpDir, "stack"), stackDir); err != nil {
		cleanup()
		return nil, fmt.Errorf("snapshot: install stack dir: %w", err)
	}
	return manifest, nil
}

// entryDest maps an archive path to its extraction location under root.
func entryDest(root, name string) (string, error) {
	switch {
	case strings.HasPrefix(name, "stack/"):
		return joinSafe(filepath.Join(root, "stack"), strings.TrimPrefix(name, "stack/"))
	case strings.HasPrefix(name, "volumes/"):
		rest := strings.TrimPrefix(name, "volumes/")
		slash := strings.Index(rest, "/")
		if slash < 0 {
			return "", badRequest("invalid volume entry: " + name)
		}
		return joinSafe(filepath.Join(root, "volumes", rest[:slash]), rest[slash+1:])
	}
	return "", badRequest("invalid archive path: " + name)
}

// joinSafe joins rel onto base, refusing anything that escapes base.
func joinSafe(base, rel string) (string, error) {
	if rel == "" || strings.HasPrefix(rel, "/") || strings.Contains(rel, `\`) {
		return "", badRequest("invalid archive path")
	}
	clean := filepath.ToSlash(filepath.Clean(rel))
	if clean != rel || clean == "." {
		return "", badRequest("invalid archive path")
	}
	dest := filepath.Join(base, filepath.FromSlash(rel))
	abs, err := filepath.Abs(dest)
	if err != nil {
		return "", badRequest("invalid archive path")
	}
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return "", badRequest("invalid archive path")
	}
	if abs != baseAbs && !strings.HasPrefix(abs, baseAbs+string(filepath.Separator)) {
		return "", badRequest("invalid archive path")
	}
	return dest, nil
}

func extractEntry(tw io.Reader, hdr *tar.Header, dest string) error {
	want := hdr.PAXRecords["ferriage.sha256"]
	if want == "" {
		return badRequest("entry missing checksum: " + hdr.Name)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("snapshot: create dir: %w", err)
	}
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("snapshot: create file: %w", err)
	}
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(f, h), tw); err != nil {
		_ = f.Close()
		return badRequest("unreadable entry: " + hdr.Name)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("snapshot: write file: %w", err)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return badRequest("checksum mismatch: " + hdr.Name)
	}
	return nil
}

// populateVolume streams the extracted volume dir (src) into the
// fresh volume full. An empty dir yields a no-op (the empty volume
// still exists, created by the caller).
func populateVolume(ctx context.Context, vols Backend, src, full string) error {
	if _, err := os.Stat(src); err != nil {
		return nil
	}
	tf, err := os.CreateTemp("", "ferriage-vol-restore-*.tar")
	if err != nil {
		return fmt.Errorf("temp volume archive: %w", err)
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
		return writeFileEntry(tw, filepath.ToSlash(rel), path, "")
	})
	if err != nil {
		_ = tw.Close()
		_ = tf.Close()
		return fmt.Errorf("tar volume dir: %w", err)
	}
	if err := tw.Close(); err != nil {
		_ = tf.Close()
		return fmt.Errorf("tar volume dir: %w", err)
	}
	if err := tf.Close(); err != nil {
		return fmt.Errorf("tar volume dir: %w", err)
	}
	in, err := os.Open(tf.Name())
	if err != nil {
		return fmt.Errorf("open volume archive: %w", err)
	}
	defer func() { _ = in.Close() }()
	if err := vols.VolumeCopyIn(ctx, full, in); err != nil {
		return fmt.Errorf("copy into volume %s: %w", full, err)
	}
	return nil
}

// safeRel validates a relative path inside a tar entry: clean, no ..,
// no backslashes.
func safeRel(rel string) error {
	if rel == "" || strings.HasPrefix(rel, "/") || strings.Contains(rel, "\\") {
		return badRequest("invalid archive path")
	}
	clean := filepath.ToSlash(filepath.Clean(rel))
	if clean != rel || clean == "." {
		return badRequest("invalid archive path")
	}
	return nil
}

// safeArcPath validates an archive path: clean relative, no .., one of
// the ADR 0002 prefixes.
func safeArcPath(p string) error {
	if p == "" || strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\`) || strings.Contains(p, `\`) {
		return badRequest("invalid archive path: " + p)
	}
	clean := filepath.ToSlash(filepath.Clean(p))
	if clean != p || clean == "." {
		return badRequest("invalid archive path: " + p)
	}
	for _, seg := range strings.Split(clean, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return badRequest("invalid archive path: " + p)
		}
	}
	if clean != manifestName && !strings.HasPrefix(clean, "stack/") && !strings.HasPrefix(clean, "volumes/") {
		return badRequest("invalid archive path: " + p)
	}
	return nil
}

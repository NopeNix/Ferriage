package snapshot

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
)

func pullOptions() image.PullOptions { return image.PullOptions{} }

// BackendFor returns a Backend backed by the daemon at DOCKER_HOST.
func BackendFor() (Backend, func(), error) {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, func() {}, fmt.Errorf("docker client: %w", err)
	}
	return &dockerBackend{cli: cli}, func() { _ = cli.Close() }, nil
}

type dockerBackend struct {
	cli *client.Client
}

func (d *dockerBackend) VolumeList(ctx context.Context, options volume.ListOptions) (volume.ListResponse, error) {
	return d.cli.VolumeList(ctx, options)
}

func (d *dockerBackend) VolumeInspect(ctx context.Context, volumeID string) (volume.Volume, error) {
	return d.cli.VolumeInspect(ctx, volumeID)
}

func (d *dockerBackend) VolumeCreate(ctx context.Context, options volume.CreateOptions) (volume.Volume, error) {
	return d.cli.VolumeCreate(ctx, options)
}

func (d *dockerBackend) VolumeRemove(ctx context.Context, volumeID string, force bool) error {
	return d.cli.VolumeRemove(ctx, volumeID, force)
}

// VolumeCopyOut copies the contents of the volume's mountpoint to w
// as a tar archive of volume-relative paths, via the daemon's
// CopyFromContainer API (a throwaway container with the volume
// bound at /vol).
func (d *dockerBackend) VolumeCopyOut(ctx context.Context, volumeName string, w io.Writer) error {
	id, err := d.sleepingContainer(ctx, helperImage, volumeName)
	if err != nil {
		return err
	}
	defer func() { _ = d.cli.ContainerRemove(ctx, id, container.RemoveOptions{Force: true}) }()
	rc, _, err := d.cli.CopyFromContainer(ctx, id, "/vol")
	if err != nil {
		return fmt.Errorf("copy volume %s: %w", volumeName, err)
	}
	defer func() { _ = rc.Close() }()
	return copyTarRebase(rc, w)
}

// VolumeCopyIn extracts the tar archive read from r into the volume,
// via the daemon's CopyToContainer API.
func (d *dockerBackend) VolumeCopyIn(ctx context.Context, volumeName string, r io.Reader) error {
	id, err := d.sleepingContainer(ctx, helperImage, volumeName)
	if err != nil {
		return err
	}
	defer func() { _ = d.cli.ContainerRemove(ctx, id, container.RemoveOptions{Force: true}) }()
	if err := d.cli.CopyToContainer(ctx, id, "/vol", r, container.CopyToContainerOptions{}); err != nil {
		return fmt.Errorf("copy into volume %s: %w", volumeName, err)
	}
	return nil
}

// sleepingContainer creates and starts a throwaway container with the
// volume bound at /vol, running `sleep infinity`, so the daemon's
// copy APIs have a container to operate on. The caller owns removal.
func (d *dockerBackend) sleepingContainer(ctx context.Context, image, volumeName string) (string, error) {
	if _, err := d.cli.ImageInspect(ctx, image); err != nil {
		rc, err := d.cli.ImagePull(ctx, image, pullOptions())
		if err != nil {
			return "", fmt.Errorf("pull %s: %w", image, err)
		}
		_, _ = io.Copy(io.Discard, rc)
		_ = rc.Close()
	}
	cfg := &container.Config{Image: image, Cmd: []string{"sleep", "infinity"}}
	host := &container.HostConfig{Binds: []string{volumeName + ":/vol"}, AutoRemove: true}
	created, err := d.cli.ContainerCreate(ctx, cfg, host, nil, nil, "")
	if err != nil {
		return "", fmt.Errorf("create helper container: %w", err)
	}
	id := created.ID
	if err := d.cli.ContainerStart(ctx, id, container.StartOptions{}); err != nil {
		_ = d.cli.ContainerRemove(ctx, id, container.RemoveOptions{Force: true})
		return "", fmt.Errorf("start helper container: %w", err)
	}
	return id, nil
}

// copyTarRebase re-emits a tar whose entries are rooted at the
// copied directory's base name (CopyFromContainer of /vol yields
// "vol/...") with volume-relative paths.
func copyTarRebase(r io.Reader, w io.Writer) error {
	tr := tar.NewReader(r)
	tw := tar.NewWriter(w)
	root := ""
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			_ = tw.Close()
			return fmt.Errorf("read volume archive: %w", err)
		}
		slashName := filepath.ToSlash(hdr.Name)
		name := strings.TrimPrefix(slashName, "./")
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
		nh := *hdr
		nh.Name = name
		if err := tw.WriteHeader(&nh); err != nil {
			_ = tw.Close()
			return fmt.Errorf("write %s: %w", name, err)
		}
		if hdr.Typeflag == tar.TypeReg {
			if _, err := io.Copy(tw, tr); err != nil {
				_ = tw.Close()
				return fmt.Errorf("copy %s: %w", name, err)
			}
		}
	}
	return tw.Close()
}

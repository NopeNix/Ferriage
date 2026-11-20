package update

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/client"
)

// dockerClient is the daemon surface the updater needs.
type dockerClient interface {
	ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error)
	ImageInspect(ctx context.Context, imageID string, opts ...client.ImageInspectOption) (image.InspectResponse, error)
	DistributionInspect(ctx context.Context, imageRef, encodedRegistryAuth string) (registry.DistributionInspect, error)
}

// DockerChecker implements DigestChecker against a real daemon.
type DockerChecker struct {
	Cli dockerClient
}

// NewDockerChecker returns the production checker (per-call client,
// like internal/stacks: no cached broken client).
func NewDockerChecker() (*DockerChecker, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, fmt.Errorf("update: docker client: %w", err)
	}
	return &DockerChecker{Cli: cli}, nil
}

// Check returns the local and remote digests for one image ref.
// An empty digest means "unresolvable" (pinned by digest, built
// locally, or never pulled) — the caller records that as
// unresolvable, never as up-to-date.
func (d *DockerChecker) Check(ctx context.Context, imageRef string) (local, remote string, err error) {
	if d.Cli == nil {
		return "", "", fmt.Errorf("no docker client")
	}
	insp, err := d.Cli.ImageInspect(ctx, imageRef)
	if err != nil {
		return "", "", fmt.Errorf("local inspect: %w", err)
	}
	// Local digest: prefer the manifest RepoDigest (what the
	// registry knows this pull as); fall back to the image ID
	// (content hash) for locally built images.
	if len(insp.RepoDigests) > 0 {
		local = insp.RepoDigests[0]
	} else if insp.ID != "" {
		local = insp.ID
	}
	remote, err = d.remoteDigest(ctx, imageRef)
	if err != nil {
		return local, "", err
	}
	return local, remote, nil
}

func (d *DockerChecker) remoteDigest(ctx context.Context, imageRef string) (string, error) {
	if isPinnedDigest(imageRef) {
		return "", fmt.Errorf("pinned by digest: %s", imageRef)
	}
	di, err := d.Cli.DistributionInspect(ctx, imageRef, "")
	if err != nil {
		return "", fmt.Errorf("registry: %w", err)
	}
	if di.Descriptor.Digest == "" {
		return "", fmt.Errorf("no manifest digest from registry")
	}
	return string(di.Descriptor.Digest), nil
}

// isPinnedDigest reports whether the reference is already a
// content-addressed digest (name@sha256:…), which can never be
// "newer" — a new digest means a new reference.
func isPinnedDigest(ref string) bool {
	return strings.Contains(ref, "@sha256:")
}

// Package adopt discovers already-running Compose stacks on the local
// Docker daemon by their labels — "adopt, don't rewrite" (AGENTS.md §3).
// It never writes to the daemon: list in, summary out.
package adopt

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// Compose label keys set by `docker compose` on every container it
// creates. Containers without these labels are not Compose stacks.
const (
	labelProject    = "com.docker.compose.project"
	labelService    = "com.docker.compose.service"
	labelConfigFile = "com.docker.compose.project.config_files"
	labelWorkingDir = "com.docker.compose.project.working_dir"
	labelOneoff     = "com.docker.compose.oneoff"
)

// Stack is one adopted Compose project: its containers grouped by the
// project label, with the compose file and working directory it was
// created from.
type Stack struct {
	Name           string    `json:"name"`
	ComposeFile    string    `json:"compose_file,omitempty"`
	WorkingDir     string    `json:"working_dir,omitempty"`
	Services       []Service `json:"services"`
	RunningCount   int       `json:"running_count"`
	ContainerCount int       `json:"container_count"`
}

// Service is one Compose service within a stack.
type Service struct {
	Name      string `json:"name"`
	Container string `json:"container"`
	Image     string `json:"image"`
	State     string `json:"state"`
	Running   bool   `json:"running"`
}

// Client reads container state from the Docker daemon.
type Client interface {
	ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error)
}

// List adopts every Compose project found among the daemon's containers.
// The result is sorted by stack name, then service name. A project is
// skipped when its containers carry no compose file label; oneoff
// containers (`docker compose run`) are excluded so transient runs do
// not appear as stacks.
func List(ctx context.Context, c Client) ([]Stack, error) {
	containers, err := c.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("adopt: list containers: %w", err)
	}
	byProject := map[string]*Stack{}
	for _, c := range containers {
		project, ok := c.Labels[labelProject]
		if !ok || project == "" {
			continue
		}
		if c.Labels[labelOneoff] == "True" {
			continue
		}
		s, ok := byProject[project]
		if !ok {
			s = &Stack{Name: project}
			byProject[project] = s
		}
		if s.ComposeFile == "" {
			s.ComposeFile = firstConfigFile(c.Labels[labelConfigFile])
		}
		if s.WorkingDir == "" {
			s.WorkingDir = c.Labels[labelWorkingDir]
		}
		s.Services = append(s.Services, Service{
			Name:      c.Labels[labelService],
			Container: strings.TrimPrefix(firstName(c.Names), "/"),
			Image:     c.Image,
			State:     string(c.State),
			Running:   c.State == container.StateRunning,
		})
		s.ContainerCount++
		if c.State == "running" {
			s.RunningCount++
		}
	}
	stacks := make([]Stack, 0, len(byProject))
	for _, s := range byProject {
		sort.Slice(s.Services, func(i, j int) bool { return s.Services[i].Name < s.Services[j].Name })
		stacks = append(stacks, *s)
	}
	sort.Slice(stacks, func(i, j int) bool { return stacks[i].Name < stacks[j].Name })
	return stacks, nil
}

func firstConfigFile(v string) string {
	for _, part := range strings.Split(v, ",") {
		if part != "" {
			return part
		}
	}
	return ""
}

func firstName(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

// NewClient returns a Docker client connected to the daemon at the
// default socket (DOCKER_HOST, else the platform default).
func NewClient() (*client.Client, error) {
	c, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, fmt.Errorf("adopt: docker client: %w", err)
	}
	return c, nil
}

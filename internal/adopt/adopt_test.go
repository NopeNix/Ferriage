package adopt

import (
	"context"
	"errors"
	"testing"

	"github.com/docker/docker/api/types/container"
)

type fakeClient struct {
	containers []container.Summary
	err        error
}

func (f fakeClient) ContainerList(context.Context, container.ListOptions) ([]container.Summary, error) {
	return f.containers, f.err
}

func TestListGroupsByProject(t *testing.T) {
	client := fakeClient{containers: []container.Summary{
		{
			Names: []string{"/web-1"},
			Image: "nginx:latest",
			State: container.StateRunning,
			Labels: map[string]string{
				labelProject: "demo", labelService: "web",
				labelConfigFile: "/srv/demo/docker-compose.yml",
				labelWorkingDir: "/srv/demo", labelOneoff: "False",
			},
		},
		{
			Names: []string{"/db-1"},
			Image: "postgres:16",
			State: container.StateExited,
			Labels: map[string]string{
				labelProject: "demo", labelService: "db",
				labelConfigFile: "/srv/demo/docker-compose.yml",
				labelWorkingDir: "/srv/demo", labelOneoff: "False",
			},
		},
		{
			Names:  []string{"/loner"},
			Image:  "alpine",
			State:  container.StateRunning,
			Labels: map[string]string{}, // no compose labels: ignored
		},
		{
			Names: []string{"/demo-run-1"},
			Image: "busybox",
			State: container.StateExited,
			Labels: map[string]string{
				labelProject: "demo", labelService: "migrate", labelOneoff: "True",
			},
		},
	}}

	stacks, err := List(context.Background(), client)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(stacks) != 1 {
		t.Fatalf("stacks = %v, want 1", stacks)
	}
	s := stacks[0]
	if s.Name != "demo" || s.ContainerCount != 2 || s.RunningCount != 1 {
		t.Errorf("stack = %+v", s)
	}
	if s.ComposeFile != "/srv/demo/docker-compose.yml" || s.WorkingDir != "/srv/demo" {
		t.Errorf("paths = %q %q", s.ComposeFile, s.WorkingDir)
	}
	if len(s.Services) != 2 || s.Services[0].Name != "db" || s.Services[1].Name != "web" {
		t.Errorf("services not sorted by name: %+v", s.Services)
	}
	if !s.Services[1].Running || s.Services[1].Container != "web-1" {
		t.Errorf("web service = %+v", s.Services[1])
	}
}

func TestListError(t *testing.T) {
	client := fakeClient{err: errors.New("boom")}
	if _, err := List(context.Background(), client); err == nil {
		t.Fatal("expected error")
	}
}

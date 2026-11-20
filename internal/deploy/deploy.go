// Package deploy brings a stack directory's Compose file into a
// healthy state on the local daemon, gating on container health.
//
// Deploy shells out to `docker compose` (the CLI the operator
// already has; compose-go is a Phase 3 evaluation, per the session
// 009 decision: no new dependency for this milestone). The gate
// itself is pure Docker Engine API: it reads each container's
// daemon-reported Health status.
package deploy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// Error is a deploy failure classified for HTTP mapping.
type Error struct {
	Code string
	Msg  string
}

func (e *Error) Error() string { return e.Msg }

func badRequest(msg string) error { return &Error{Code: "bad_request", Msg: msg} }
func internal(msg string) error   { return &Error{Code: "internal", Msg: msg} }

// Result is a deploy outcome: per-service health after the gate.
type Result struct {
	Stack    string `json:"stack"`
	Services []Svc  `json:"services"`
	Healthy  bool   `json:"healthy"`
	Detail   string `json:"detail,omitempty"`
}

// Svc is one service's gated state.
type Svc struct {
	Name      string `json:"name"`
	Health    string `json:"health"`
	HasHealth bool   `json:"has_healthcheck"`
	Running   bool   `json:"running"`
}

// Config tunes the deploy gate.
type Config struct {
	// Compose is the compose CLI path ("docker" → `docker compose`).
	Compose string
	// Timeout bounds the up call and the health gate together.
	Timeout time.Duration
	// Poll is the health-gate poll interval.
	Poll time.Duration
}

// DefaultConfig is the production tuning.
func DefaultConfig() Config {
	return Config{Compose: "docker", Timeout: 120 * time.Second, Poll: 2 * time.Second}
}

// composeCmd builds the `docker compose` invocation for a stack dir.
func composeCmd(compose, stackDir string, args ...string) *exec.Cmd {
	full := append([]string{"compose", "-f", "compose.yaml", "-p", filepath.Base(stackDir)}, args...)
	return exec.Command(compose, full...)
}

// errText caps captured CLI stderr for error messages.
func errText(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 500 {
		s = s[:500] + "…"
	}
	return s
}

// Deploy runs `docker compose up -d` for the stack in stackDir, then
// polls the daemon until every health-checked container is healthy
// (or the timeout expires). Containers without a healthcheck count
// as healthy once running. On gate failure the result still names
// the returning services; the stack is left running (the operator
// decides; Ferriage never tears down a failed deploy on its own).
func Deploy(ctx context.Context, stackDir string, cfg Config) (*Result, error) {
	if cfg.Compose == "" {
		cfg.Compose = "docker"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 120 * time.Second
	}
	if cfg.Poll <= 0 {
		cfg.Poll = 2 * time.Second
	}

	upCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	var stderr bytes.Buffer
	cmd := composeCmd(cfg.Compose, stackDir, "up", "-d")
	cmd.Dir = stackDir
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if upCtx.Err() == context.DeadlineExceeded {
			return nil, internal("docker compose up timed out")
		}
		return nil, internal(fmt.Sprintf("docker compose up: %v: %s", err, errText(stderr.Bytes())))
	}

	return gate(upCtx, stackDir, cfg)
}

// gate polls container health until all health-checked containers
// are healthy; on context expiry it returns the last observed state
// as a bad-request error so the caller can report what was sick.
func gate(ctx context.Context, stackDir string, cfg Config) (*Result, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, internal(fmt.Sprintf("docker client: %v", err))
	}
	defer func() { _ = cli.Close() }()

	project := filepath.Base(stackDir)
	for {
		svcs, err := listHealth(ctx, cli, project)
		if err != nil {
			return nil, internal(err.Error())
		}
		res := &Result{Stack: project, Services: svcs, Healthy: allHealthy(svcs)}
		if res.Healthy {
			return res, nil
		}
		select {
		case <-ctx.Done():
			res.Detail = "health gate timed out: " + unhealthyNames(svcs)
			return res, badRequest(res.Detail)
		case <-time.After(cfg.Poll):
		}
	}
}

func allHealthy(svcs []Svc) bool {
	if len(svcs) == 0 {
		return false
	}
	for _, s := range svcs {
		if !s.Running {
			return false
		}
		if s.HasHealth && s.Health != "healthy" {
			return false
		}
	}
	return true
}

// unhealthyNames names services that are not yet healthy.
func unhealthyNames(svcs []Svc) string {
	var out []string
	for _, s := range svcs {
		if !s.Running || (s.HasHealth && s.Health != "healthy") {
			out = append(out, fmt.Sprintf("%s(%s)", s.Name, s.Health))
		}
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

// listHealth returns one Svc per compose service of the project,
// from daemon container summaries.
func listHealth(ctx context.Context, cli *client.Client, project string) ([]Svc, error) {
	containers, err := cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	var svcs []Svc
	for _, c := range containers {
		if c.Labels["com.docker.compose.project"] != project {
			continue
		}
		if c.Labels["com.docker.compose.oneoff"] == "True" {
			continue
		}
		svc := Svc{
			Name:    c.Labels["com.docker.compose.service"],
			Running: c.State == container.StateRunning,
		}
		if c.State == container.StateRunning {
			svc.Health = "running"
			svc.HasHealth = false
		}
		// Health status needs an inspect; summaries don't carry it.
		details, err := cli.ContainerInspect(ctx, c.ID)
		if err == nil && details.State != nil {
			if details.State.Health != nil {
				svc.HasHealth = true
				svc.Health = details.State.Health.Status
			}
		}
		svcs = append(svcs, svc)
	}
	sort.Slice(svcs, func(i, j int) bool { return svcs[i].Name < svcs[j].Name })
	return svcs, nil
}

// Report is the JSON body of the deploy response.
func Report(r *Result) []byte {
	b, _ := json.Marshal(r)
	return b
}

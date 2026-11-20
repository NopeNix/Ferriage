// Package stats serves one-shot live resource snapshots for a
// stack's containers (Phase 3 item 2). Like logs, stats are
// transport artifacts, not state (ADR 0005).
package stats

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"

	"github.com/NopeNix/Ferriage/internal/adopt"
	"github.com/NopeNix/Ferriage/internal/api"
	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
)

// Client talks to the Docker daemon for stats and container lookup.
type Client interface {
	ContainerStatsOneShot(ctx context.Context, containerID string) (container.StatsResponseReader, error)
	ContainerInspect(ctx context.Context, containerID string) (container.InspectResponse, error)
}

// Snapshot is one container's live resource snapshot.
type Snapshot struct {
	Container     string
	CPUPercent    float64
	MemoryBytes   uint64
	MemoryLimit   uint64
	MemoryPercent float64
	NetRxBytes    uint64
	NetTxBytes    uint64
}

// cpuPercent computes CPU% from two consecutive stats readings,
// per Docker's `docker stats` formula.
func cpuPercent(s container.StatsResponse) float64 {
	if s.PreCPUStats.SystemUsage == 0 || s.CPUStats.SystemUsage == 0 {
		return 0
	}
	cpuDelta := float64(s.CPUStats.CPUUsage.TotalUsage - s.PreCPUStats.CPUUsage.TotalUsage)
	sysDelta := float64(s.CPUStats.SystemUsage - s.PreCPUStats.SystemUsage)
	if sysDelta <= 0 {
		return 0
	}
	online := float64(s.CPUStats.OnlineCPUs)
	if online == 0 {
		online = 1
	}
	return (cpuDelta / sysDelta) * online * 100
}

// snapshotOne reads one stats response and converts it.
func snapshotOne(r container.StatsResponseReader) (Snapshot, error) {
	dec := json.NewDecoder(r.Body)
	var s container.StatsResponse
	if err := dec.Decode(&s); err != nil {
		return Snapshot{}, err
	}
	var rx, tx uint64
	for _, n := range s.Networks {
		rx += n.RxBytes
		tx += n.TxBytes
	}
	var memPct float64
	if s.MemoryStats.Limit > 0 {
		memPct = float64(s.MemoryStats.Usage) / float64(s.MemoryStats.Limit) * 100
	}
	return Snapshot{
		Container:     s.Name,
		CPUPercent:    cpuPercent(s),
		MemoryBytes:   s.MemoryStats.Usage,
		MemoryLimit:   s.MemoryStats.Limit,
		MemoryPercent: memPct,
		NetRxBytes:    rx,
		NetTxBytes:    tx,
	}, nil
}

// collect gathers one-shot snapshots for every stack container,
// sorted by container name. Containers whose stats fail are
// reported as zero snapshots rather than failing the whole call.
func Collect(ctx context.Context, c Client, containers []string) ([]Snapshot, error) {
	var out []Snapshot
	for _, id := range containers {
		r, err := c.ContainerStatsOneShot(ctx, id)
		if err != nil {
			if errdefs.IsNotFound(err) {
				continue
			}
			out = append(out, Snapshot{Container: id})
			continue
		}
		snap, serr := snapshotOne(r)
		_ = r.Body.Close()
		if serr != nil {
			out = append(out, Snapshot{Container: id})
			continue
		}
		if snap.Container == "" {
			snap.Container = id
		}
		out = append(out, snap)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Container < out[j].Container })
	return out, nil
}

// Handler serves GET /stacks/{name}/stats.
type Handler struct {
	Lister    func(context.Context) ([]adopt.Stack, error)
	NewClient func() (Client, error)
}

// Register wires the stats route onto the peer mux.
func Register(mux *http.ServeMux) {
	h := &Handler{
		Lister: func(ctx context.Context) ([]adopt.Stack, error) {
			c, err := adopt.NewClient()
			if err != nil {
				return nil, err
			}
			defer func() { _ = c.Close() }()
			return adopt.List(ctx, c)
		},
		NewClient: func() (Client, error) { return adopt.NewClient() },
	}
	mux.HandleFunc("GET /stacks/{name}/stats", h.serve)
}

func (h *Handler) serve(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !validName(name) {
		writeErr(w, http.StatusBadRequest, "bad stack name")
		return
	}
	stacks, err := h.Lister(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	var containers []string
	found := false
	for _, s := range stacks {
		if s.Name != name {
			continue
		}
		found = true
		for _, sv := range s.Services {
			containers = append(containers, sv.Container)
		}
	}
	if !found {
		writeErr(w, http.StatusNotFound, fmt.Sprintf("stack %q not found", name))
		return
	}
	c, err := h.NewClient()
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	defer func() {
		if closer, ok := any(c).(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	}()
	snaps, err := Collect(r.Context(), c, containers)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	out := make([]api.ContainerStats, 0, len(snaps))
	for _, s := range snaps {
		out = append(out, api.ContainerStats{
			Container:     s.Container,
			CPUPercent:    s.CPUPercent,
			MemoryBytes:   s.MemoryBytes,
			MemoryLimit:   s.MemoryLimit,
			MemoryPercent: s.MemoryPercent,
			NetRxBytes:    s.NetRxBytes,
			NetTxBytes:    s.NetTxBytes,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(api.StatsResponse{Stats: out})
}

func validName(s string) bool {
	if s == "" || s == "." || s == ".." {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
		default:
			return false
		}
	}
	return true
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(api.ErrorResponse{Error: msg})
}

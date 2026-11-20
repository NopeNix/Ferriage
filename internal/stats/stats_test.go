package stats

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NopeNix/Ferriage/internal/adopt"
	"github.com/docker/docker/api/types/container"
)

type fakeStatsClient struct {
	byID map[string]container.StatsResponse
}

func (f *fakeStatsClient) ContainerStatsOneShot(_ context.Context, id string) (container.StatsResponseReader, error) {
	s, ok := f.byID[id]
	if !ok {
		s = container.StatsResponse{}
	}
	b, _ := json.Marshal(s)
	return container.StatsResponseReader{Body: io.NopCloser(strings.NewReader(string(b)))}, nil
}

func (f *fakeStatsClient) ContainerInspect(context.Context, string) (container.InspectResponse, error) {
	return container.InspectResponse{}, nil
}

func mk(cpu, sys, preCpu, preSys, mem, memLimit uint64) container.StatsResponse {
	s := container.StatsResponse{}
	s.CPUStats.CPUUsage.TotalUsage = cpu
	s.CPUStats.SystemUsage = sys
	s.PreCPUStats.CPUUsage.TotalUsage = preCpu
	s.PreCPUStats.SystemUsage = preSys
	s.CPUStats.OnlineCPUs = 1
	s.MemoryStats.Usage = mem
	s.MemoryStats.Limit = memLimit
	s.Networks = map[string]container.NetworkStats{"eth0": {RxBytes: 10, TxBytes: 20}}
	return s
}

func TestCPUPercent(t *testing.T) {
	// 50/100 over 1 online CPU = 50%
	if got := cpuPercent(mk(150, 300, 100, 200, 0, 0)); got != 50 {
		t.Fatalf("got %v", got)
	}
	if got := cpuPercent(mk(0, 0, 0, 0, 0, 0)); got != 0 {
		t.Fatalf("zero sys: got %v", got)
	}
}

func TestCollectSortsAndWraps(t *testing.T) {
	f := &fakeStatsClient{byID: map[string]container.StatsResponse{
		"b": mk(150, 300, 100, 200, 50, 100),
		"a": mk(0, 0, 0, 0, 25, 100),
	}}
	snaps, err := Collect(context.Background(), f, []string{"b", "a"})
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 2 || snaps[0].Container != "a" {
		t.Fatalf("sort: %+v", snaps)
	}
	if snaps[1].NetRxBytes != 10 || snaps[1].NetTxBytes != 20 {
		t.Fatalf("net: %+v", snaps[1])
	}
	if snaps[0].MemoryPercent != 25 {
		t.Fatalf("mem pct: %+v", snaps[0])
	}
}

func TestHandler404AndOK(t *testing.T) {
	h := &Handler{
		Lister: func(context.Context) ([]adopt.Stack, error) {
			return []adopt.Stack{{Name: "s", Services: []adopt.Service{{Name: "app", Container: "cid"}}}}, nil
		},
		NewClient: func() (Client, error) {
			return &fakeStatsClient{byID: map[string]container.StatsResponse{"cid": mk(150, 300, 100, 200, 50, 100)}}, nil
		},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /stacks/{name}/stats", h.serve)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/stacks/ghost/stats", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown stack: %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/stacks/s/stats", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"cpu_percent":50`) {
		t.Fatalf("ok: %d %s", rec.Code, rec.Body.String())
	}
}

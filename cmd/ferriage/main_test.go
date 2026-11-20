package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NopeNix/Ferriage/internal/api"
)

// fakePeer serves the typed API contract with canned responses.
func fakePeer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /peer", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(api.PeerResponse{PeerID: "p1", Version: "dev", Commit: "abc"})
	})
	mux.HandleFunc("GET /stacks", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(api.StacksResponse{Stacks: []api.Stack{
			{Name: "demo", Services: []api.Service{{Name: "app", Container: "demo-app-1", Image: "alpine:3.20", Running: true}}, RunningCount: 1, ContainerCount: 1},
		}})
	})
	mux.HandleFunc("POST /stacks/{name}/deploy", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(api.DeployResponse{
			Stack:    r.PathValue("name"),
			Healthy:  true,
			Services: []api.DeploySvc{{Name: "app", Health: "healthy", HasHealth: true, Running: true}},
		})
	})
	mux.HandleFunc("POST /stacks/{name}/export", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write([]byte("gzip-bytes"))
	})
	mux.HandleFunc("POST /stacks/{name}/import", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(api.SnapshotManifest{
			FormatVersion: 1, Stack: r.PathValue("name"), Files: []string{"compose.yaml"}, Volumes: []api.VolumeDesc{{Name: "demo_data"}},
		})
	})
	mux.HandleFunc("POST /stacks/missing/deploy", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(api.ErrorResponse{Error: "stack not found"})
	})
	return httptest.NewServer(mux)
}

func TestStacksHumanAndJSON(t *testing.T) {
	srv := fakePeer(t)
	defer srv.Close()

	var out bytes.Buffer
	if err := run([]string{"--peer", srv.URL, "stacks"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "demo  1/1 running") {
		t.Errorf("human stacks output = %q", out.String())
	}

	out.Reset()
	if err := run([]string{"--peer", srv.URL, "--json", "stacks"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var resp api.StacksResponse
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("json stacks = %q: %v", out.String(), err)
	}
	if len(resp.Stacks) != 1 || resp.Stacks[0].Name != "demo" {
		t.Errorf("stacks = %+v", resp.Stacks)
	}
}

func TestPeerHumanAndJSON(t *testing.T) {
	srv := fakePeer(t)
	defer srv.Close()
	var out bytes.Buffer
	if err := run([]string{"--peer", srv.URL, "peer"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "peer p1") {
		t.Errorf("peer output = %q", out.String())
	}
	out.Reset()
	if err := run([]string{"--peer", srv.URL, "--json", "peer"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var resp api.PeerResponse
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil || resp.PeerID != "p1" {
		t.Errorf("json peer = %q, err %v", out.String(), err)
	}
}

func TestDeploy(t *testing.T) {
	srv := fakePeer(t)
	defer srv.Close()
	var out bytes.Buffer
	if err := run([]string{"--peer", srv.URL, "deploy", "demo"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "demo: healthy") {
		t.Errorf("deploy output = %q", out.String())
	}
	out.Reset()
	if err := run([]string{"--peer", srv.URL, "--json", "deploy", "demo"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var resp api.DeployResponse
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil || !resp.Healthy || resp.Stack != "demo" {
		t.Errorf("json deploy = %+v, err %v", resp, err)
	}
}

func TestErrorSurfaces(t *testing.T) {
	srv := fakePeer(t)
	defer srv.Close()
	err := run([]string{"--peer", srv.URL, "deploy", "missing"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "stack not found") || !strings.Contains(err.Error(), "HTTP 404") {
		t.Errorf("err = %v", err)
	}
}

func TestUsageAndVersion(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"--help"}, &out, &bytes.Buffer{}); err != nil || !strings.Contains(out.String(), "ferriage") {
		t.Errorf("help = %q, err %v", out.String(), err)
	}
	out.Reset()
	if err := run([]string{"version"}, &out, &bytes.Buffer{}); err != nil || !strings.Contains(out.String(), "ferriage") {
		t.Errorf("version = %q, err %v", out.String(), err)
	}
	if err := run([]string{"bogus"}, &out, &bytes.Buffer{}); err == nil {
		t.Error("bogus command should fail")
	}
	if err := run([]string{"deploy"}, &out, &bytes.Buffer{}); err == nil {
		t.Error("deploy without a stack should fail")
	}
}

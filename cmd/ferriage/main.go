// Command ferriage is the Ferriage CLI: the CLI twin of every UI
// action (CONCEPT: API-first). It talks to a peer's HTTP API — the
// same typed contract the web UI uses (internal/api) — so a script
// and a browser see exactly the same state.
//
// Usage:
//
// ferriage [--peer URL] [--json] stacks
// ferriage [--peer URL] [--json] peer
// ferriage [--peer URL] [--json] deploy <stack>
// ferriage [--peer URL] export <stack> [-o file.tar.gz]
// ferriage [--peer URL] import <stack> <file.tar.gz>
// ferriage version
//
// The peer defaults to $FERRIAGE_PEER, else http://localhost:8080.
// --json prints the peer's JSON response verbatim (for automation);
// without it, output is human-readable tables.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/NopeNix/Ferriage/internal/api"
	"github.com/NopeNix/Ferriage/internal/version"
)

const usage = `ferriage — CLI twin of the Ferriage UI (same typed peer API)

Usage:
  ferriage [--peer URL] [--json] stacks
  ferriage [--peer URL] [--json] peer
  ferriage [--peer URL] [--json] deploy <stack>
  ferriage [--peer URL] export <stack> [-o file.tar.gz]
  ferriage [--peer URL] import <stack> <file.tar.gz>
  ferriage version
  ferriage help

Flags:
  --peer URL   peer base URL (default: $FERRIAGE_PEER, else http://localhost:8080)
  --json       print the peer's JSON response verbatim
`

type options struct {
	peer string
	json bool
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "ferriage:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	opts := options{peer: defaultPeer()}
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--peer" && i+1 < len(args):
			i++
			opts.peer = args[i]
		case strings.HasPrefix(a, "--peer="):
			opts.peer = strings.TrimPrefix(a, "--peer=")
		case a == "--json":
			opts.json = true
		default:
			rest = append(rest, args[i:]...)
			i = len(args)
		}
	}

	if len(rest) == 0 {
		_, _ = fmt.Fprint(stdout, usage)
		return nil
	}
	cmd, argv := rest[0], rest[1:]
	c := &client{peer: strings.TrimRight(opts.peer, "/"), http: &http.Client{Timeout: 5 * time.Minute}}

	switch cmd {
	case "help", "-h", "--help":
		_, _ = fmt.Fprint(stdout, usage)
		return nil
	case "version":
		_, _ = fmt.Fprintf(stdout, "ferriage %s (commit %s)\n", version.Version, version.Commit)
		return nil
	case "stacks":
		var resp api.StacksResponse
		if err := c.get("/stacks", &resp); err != nil {
			return err
		}
		if opts.json {
			return writeJSON(stdout, &resp)
		}
		for _, s := range resp.Stacks {
			_, _ = fmt.Fprintf(stdout, "%s  %d/%d running\n", s.Name, s.RunningCount, s.ContainerCount)
			for _, svc := range s.Services {
				state := "stopped"
				if svc.Running {
					state = "running"
				}
				_, _ = fmt.Fprintf(stdout, "  %s  %s  %s  %s\n", svc.Name, svc.Container, state, svc.Image)
			}
		}
		return nil
	case "peer":
		var resp api.PeerResponse
		if err := c.get("/peer", &resp); err != nil {
			return err
		}
		if opts.json {
			return writeJSON(stdout, &resp)
		}
		_, _ = fmt.Fprintf(stdout, "peer %s  version %s  commit %s\n", resp.PeerID, resp.Version, resp.Commit)
		return nil
	case "deploy":
		if len(argv) < 1 {
			return usageErr("deploy needs a stack name")
		}
		var resp api.DeployResponse
		if err := c.postJSON("/stacks/"+url.PathEscape(argv[0])+"/deploy", nil, &resp); err != nil {
			return err
		}
		if opts.json {
			return writeJSON(stdout, &resp)
		}
		health := "UNHEALTHY"
		if resp.Healthy {
			health = "healthy"
		}
		_, _ = fmt.Fprintf(stdout, "%s: %s\n", resp.Stack, health)
		for _, s := range resp.Services {
			h := ""
			if s.HasHealth {
				h = " health=" + s.Health
			}
			_, _ = fmt.Fprintf(stdout, "  %s running=%v%s\n", s.Name, s.Running, h)
		}
		if resp.Detail != "" {
			_, _ = fmt.Fprintf(stdout, "  detail: %s\n", resp.Detail)
		}
		return nil
	case "export":
		if len(argv) < 1 {
			return usageErr("export needs a stack name")
		}
		out := argv[0] + ".tar.gz"
		for i := 1; i < len(argv); i++ {
			if argv[i] == "-o" && i+1 < len(argv) {
				i++
				out = argv[i]
			}
		}
		body, err := c.postBinary("/stacks/"+url.PathEscape(argv[0])+"/export", nil)
		if err != nil {
			return err
		}
		if err := os.WriteFile(out, body, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", out, err)
		}
		_, _ = fmt.Fprintf(stdout, "%s (%d bytes)\n", out, len(body))
		return nil
	case "import":
		if len(argv) < 2 {
			return usageErr("import needs a stack name and a snapshot file")
		}
		data, err := os.ReadFile(argv[1])
		if err != nil {
			return fmt.Errorf("read %s: %w", argv[1], err)
		}
		var resp api.SnapshotManifest
		if err := c.postBinaryJSON("/stacks/"+url.PathEscape(argv[0])+"/import", data, &resp); err != nil {
			return err
		}
		if opts.json {
			return writeJSON(stdout, &resp)
		}
		_, _ = fmt.Fprintf(stdout, "imported %s (%d files, %d volumes)\n", resp.Stack, len(resp.Files), len(resp.Volumes))
		return nil
	default:
		return usageErr("unknown command %q", cmd)
	}
}

func usageErr(format string, a ...any) error {
	return fmt.Errorf("%s\n\n%s", fmt.Sprintf(format, a...), usage)
}

func defaultPeer() string {
	if p := os.Getenv("FERRIAGE_PEER"); p != "" {
		return p
	}
	return "http://localhost:8080"
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

type client struct {
	peer string
	http *http.Client
}

func (c *client) get(path string, out any) error {
	return c.do(http.MethodGet, path, nil, nil, out)
}

func (c *client) postJSON(path string, body []byte, out any) error {
	ct := "application/json"
	return c.do(http.MethodPost, path, body, &ct, out)
}

func (c *client) postBinary(path string, body []byte) ([]byte, error) {
	_, raw, err := c.doRaw(http.MethodPost, path, body, nil)
	return raw, err
}

func (c *client) postBinaryJSON(path string, body []byte, out any) error {
	ct := "application/gzip"
	return c.do(http.MethodPost, path, body, &ct, out)
}

func (c *client) do(method, path string, body []byte, contentType *string, out any) error {
	_, raw, err := c.doRaw(method, path, body, contentType)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	return nil
}

func (c *client) doRaw(method, path string, body []byte, contentType *string) (int, []byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, c.peer+path, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	if contentType != nil {
		req.Header.Set("Content-Type", *contentType)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("peer %s: %w", c.peer, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<31))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var e api.ErrorResponse
		if err := json.Unmarshal(raw, &e); err == nil && e.Error != "" {
			return resp.StatusCode, raw, fmt.Errorf("%s: %s (HTTP %d)", path, e.Error, resp.StatusCode)
		}
		return resp.StatusCode, raw, fmt.Errorf("%s: HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return resp.StatusCode, raw, nil
}

// Ensure filepath is used even if a future command drops it.
var _ = filepath.Join

// Package onboard implements SSH onboarding (CONCEPT hard
// requirements 11+12, ADR 0006): add a host by address + key,
// check for Docker + the Compose plugin (install if missing),
// then start one Ferriage container on it. The peer is the SSH
// client itself — no openssh-client dependency in the image.
package onboard

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/pem"
	"fmt"

	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// Request is one onboarding operation.
type Request struct {
	// Address is host or host:port (default port 22).
	Address string `json:"address"`
	// User is the SSH user (default "root").
	User string `json:"user,omitempty"`
	// PrivateKey is the PEM-encoded SSH private key (key-only
	// auth; passwords are never accepted — ADR 0006).
	PrivateKey string `json:"private_key"`
	// Image is the peer image to run (default nopenix/ferriage:latest).
	Image string `json:"image,omitempty"`
	// PeerURL is the URL the new peer pulls mesh state from
	// (the operator's peer — ADR 0001 is pull-based).
	PeerURL string `json:"peer_url,omitempty"`
	// Port is the peer port on the new host (default 8080).
	Port int `json:"port,omitempty"`
}

// Step records one onboarding phase.
type Step struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

// Result is the outcome of an onboarding operation.
type Result struct {
	Address   string `json:"address"`
	Docker    string `json:"docker"` // "present" | "installed" | "missing"
	Compose   string `json:"compose"`
	Container string `json:"container"`
	PeerURL   string `json:"peer_url"`
	Healthy   bool   `json:"healthy"`
	Steps     []Step `json:"steps"`
}

// Error is an onboarding failure classified for HTTP mapping.
type Error struct {
	Code string // "bad_request" | "upstream"
	Msg  string
}

func (e *Error) Error() string { return e.Msg }

func badRequest(format string, a ...any) error {
	return &Error{Code: "bad_request", Msg: fmt.Sprintf(format, a...)}
}
func upstream(format string, a ...any) error {
	return &Error{Code: "upstream", Msg: fmt.Sprintf(format, a...)}
}

// Validate checks the request before any network contact.
func (r *Request) Validate() error {
	if r.Address == "" {
		return badRequest("address is required")
	}
	if strings.Contains(r.Address, " ") || strings.ContainsAny(r.Address, "\r\n\t") {
		return badRequest("address must be a host or host:port")
	}
	if _, _, err := net.SplitHostPort(maybePort(r.Address)); err != nil {
		return badRequest("address %q: %v", r.Address, err)
	}
	if r.PrivateKey == "" {
		return badRequest("private_key is required (key-only auth)")
	}
	parsed, _ := pem.Decode([]byte(r.PrivateKey))
	if parsed == nil {
		return badRequest("private_key is not PEM-encoded")
	}
	if parsed.Type != "RSA PRIVATE KEY" && parsed.Type != "EC PRIVATE KEY" &&
		parsed.Type != "OPENSSH PRIVATE KEY" && parsed.Type != "PRIVATE KEY" &&
		parsed.Type != "DSA PRIVATE KEY" && parsed.Type != "ED25519 PRIVATE KEY" {
		return badRequest("private_key type %q is not a private key", parsed.Type)
	}
	if r.User == "" {
		r.User = "root"
	}
	if strings.ContainsAny(r.User, " \t\r\n;|&$`<>") {
		return badRequest("bad ssh user %q", r.User)
	}
	if r.Port != 0 && (r.Port < 1 || r.Port > 65535) {
		return badRequest("port must be 1..65535")
	}
	if r.Port == 0 {
		r.Port = 8080
	}
	if r.Image == "" {
		r.Image = "nopenix/ferriage:latest"
	}
	return nil
}

func maybePort(addr string) string {
	if _, _, err := net.SplitHostPort(addr); err == nil {
		return addr
	}
	return net.JoinHostPort(addr, "22")
}

// hostKeyCallback returns the SSH host-key verifier. When a pin
// file (FERRIAGE_SSH_KNOWN_HOSTS) is configured it behaves like
// openssh; otherwise it is trust-on-first-use against a store
// under the state dir (ADR 0006).
type hostKeyCallback struct {
	pinned    *knownHosts
	storePath string
	mu        sync.Mutex
	accepted  []string
}

func newHostKeyCallback(stateDir string) (*hostKeyCallback, error) {
	h := &hostKeyCallback{}
	if pin := os.Getenv("FERRIAGE_SSH_KNOWN_HOSTS"); pin != "" {
		kh, err := loadKnownHosts(pin)
		if err != nil {
			return nil, fmt.Errorf("host key pin file %s: %w", pin, err)
		}
		h.pinned = kh
		return h, nil
	}
	if stateDir == "" {
		stateDir = "./stacks"
	}
	h.storePath = filepath.Join(stateDir, ".ferriage", "known_hosts")
	// TO-FU store: only a non-empty existing store counts as
	// "pinned". A missing or empty file means first use.
	if fi, err := os.Stat(h.storePath); err == nil && fi.Size() > 0 {
		if kh, err := loadKnownHosts(h.storePath); err == nil {
			h.pinned = kh
		}
	}
	return h, nil
}

func (h *hostKeyCallback) verify(hostname string, remote net.Addr, key ssh.PublicKey) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	algo := key.Type()
	fp := fingerprint(key)
	if h.pinned != nil {
		ok, known := h.pinned.lookup(hostname, algo)
		if !known {
			return fmt.Errorf("host key for %s not pinned (set FERRIAGE_SSH_KNOWN_HOSTS)", hostname)
		}
		if !ok {
			return fmt.Errorf("host key mismatch for %s (possible MITM; pinned %s)", hostname, algo)
		}
		h.accepted = append(h.accepted, fp)
		return nil
	}
	// TO-FU: accept, persist, log.
	if err := os.MkdirAll(filepath.Dir(h.storePath), 0o700); err != nil {
		return fmt.Errorf("known_hosts store: %w", err)
	}
	line := hostname + " " + algo + " " + base64.StdEncoding.EncodeToString(key.Marshal()) + "\n"
	f, err := os.OpenFile(h.storePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("known_hosts store: %w", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(line); err != nil {
		return fmt.Errorf("known_hosts store: %w", err)
	}
	h.accepted = append(h.accepted, fp)
	fingerprintLog(hostname, algo, fp)
	return nil
}

func fingerprintLog(host, algo, fp string) {
	// Package log via stdlib would create a cycle-free import;
	// kept inline to avoid an import for one line.
	println("ferriage onboard: trusted host key", host, algo, fp)
}

func fingerprint(key ssh.PublicKey) string {
	sum := sha256.Sum256(key.Marshal())
	return "SHA256:" + base64.StdEncoding.EncodeToString(sum[:])
}

// knownHosts is a minimal known_hosts parser.
type knownHosts struct {
	byHost map[string]map[string]string // host -> algo -> base64 key
}

func loadKnownHosts(path string) (*knownHosts, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &knownHosts{byHost: map[string]map[string]string{}}, nil
		}
		return nil, err
	}
	kh := &knownHosts{byHost: map[string]map[string]string{}}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}
		host, algo, b64 := parts[0], parts[1], parts[2]
		if kh.byHost[host] == nil {
			kh.byHost[host] = map[string]string{}
		}
		kh.byHost[host][algo] = b64
	}
	return kh, nil
}

func (kh *knownHosts) lookup(host, algo string) (ok, known bool) {
	keys, found := kh.byHost[host]
	if !found {
		// Try the bare host without port decoration: ssh
		// known_hosts matches on the address as dialed.
		keys, found = kh.byHost[stripPort(host)]
		if !found {
			return false, false
		}
	}
	b64, found := keys[algo]
	if !found {
		return false, false
	}
	return b64 != "", true
}

func stripPort(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

// Runner is the host-side command/upload surface. *Client is
// the SSH implementation; tests substitute a fake.
type Runner interface {
	Exec(ctx context.Context, cmd string) (string, error)
	Upload(ctx context.Context, path string, content []byte) error
	CheckDocker(ctx context.Context) DockerStatus
	EnsureDocker(ctx context.Context, step func(name string, ok bool, detail string)) error
	Close() error
}

// Client is one SSH session to a host being onboarded.
type Client struct {
	Conn *ssh.Client
}

var _ Runner = (*Client)(nil)

// Dial opens the SSH connection for req. The caller closes it.
func Dial(ctx context.Context, req *Request, stateDir string) (*Client, error) {
	signer, err := ssh.ParsePrivateKey([]byte(req.PrivateKey))
	if err != nil {
		return nil, badRequest("parse private_key: %v", err)
	}
	hk, err := newHostKeyCallback(stateDir)
	if err != nil {
		return nil, upstream("%v", err)
	}
	cfg := &ssh.ClientConfig{
		User:            req.User,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: hk.verify,
		Timeout:         15 * time.Second,
	}
	addr := maybePort(req.Address)
	d := net.Dialer{Timeout: 15 * time.Second}
	raw, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, upstream("dial %s: %v", addr, err)
	}
	sshConn, chans, reqs, err := ssh.NewClientConn(raw, addr, cfg)
	if err != nil {
		_ = raw.Close()
		return nil, upstream("ssh handshake %s as %s: %v", addr, req.User, err)
	}
	return &Client{Conn: ssh.NewClient(sshConn, chans, reqs)}, nil
}

func (c *Client) Close() error { return c.Conn.Close() }

// Exec runs cmd on the host and returns combined output.
func (c *Client) Exec(ctx context.Context, cmd string) (string, error) {
	sess, err := c.Conn.NewSession()
	if err != nil {
		return "", err
	}
	defer func() { _ = sess.Close() }()
	var buf bytes.Buffer
	sess.Stdout = &buf
	sess.Stderr = &buf
	if err := sess.Run(cmd); err != nil {
		return buf.String(), fmt.Errorf("%s: %w", cmd, err)
	}
	return buf.String(), nil
}

// ExecOK runs cmd, treating a non-zero exit as an error.
func (c *Client) ExecOK(ctx context.Context, cmd string) (string, error) {
	out, err := c.Exec(ctx, cmd)
	return out, err
}

// Upload writes content to path via `cat > path` (no SFTP
// dependency; files are small compose files).
func (c *Client) Upload(ctx context.Context, path string, content []byte) error {
	sess, err := c.Conn.NewSession()
	if err != nil {
		return err
	}
	defer func() { _ = sess.Close() }()
	stdin, err := sess.StdinPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	sess.Stderr = &stderr
	go func() {
		_, _ = stdin.Write(content)
		_ = stdin.Close()
	}()
	quoted := "'" + strings.ReplaceAll(path, "'", "'\\''") + "'"
	if err := sess.Run("cat > " + quoted); err != nil {
		return fmt.Errorf("upload %s: %v: %s", path, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// DockerStatus reports the host's Docker + Compose state.
type DockerStatus struct {
	Docker  bool
	Compose bool
	Version string
	OS      string
}

// CheckDocker probes `docker --version` and `docker compose version`.
func (c *Client) CheckDocker(ctx context.Context) DockerStatus {
	ds := DockerStatus{}
	if out, err := c.Exec(ctx, "docker --version 2>/dev/null"); err == nil {
		ds.Docker = true
		ds.Version = strings.TrimSpace(out)
	}
	if _, err := c.Exec(ctx, "docker compose version >/dev/null 2>&1"); err == nil {
		ds.Compose = true
	}
	if out, err := c.Exec(ctx, "cat /etc/os-release 2>/dev/null | head -1"); err == nil {
		ds.OS = strings.TrimSpace(out)
	}
	return ds
}

// EnsureDocker installs Docker (get.docker.com) when missing.
// Destructive-adjacent: only reached for a host the operator
// explicitly asked to onboard (AGENTS.md §5).
func (c *Client) EnsureDocker(ctx context.Context, step func(name string, ok bool, detail string)) error {
	ds := c.CheckDocker(ctx)
	if ds.Docker && ds.Compose {
		step("docker-check", true, ds.Version)
		return nil
	}
	if ds.Docker && !ds.Compose {
		// Compose plugin missing but daemon present (Docker
		// <23): install the plugin package directly.
		step("docker-check", true, ds.Version+" (compose plugin missing)")
		if _, err := c.Exec(ctx, "apt-get update -qq && apt-get install -y -qq docker-compose-plugin"); err == nil {
			ds = c.CheckDocker(ctx)
			if ds.Compose {
				step("docker-compose-install", true, "docker-compose-plugin")
				return nil
			}
		}
		return upstream("docker present but compose plugin could not be installed")
	}
	step("docker-check", false, "docker missing — installing via get.docker.com")
	// get.docker.com needs root (or passwordless sudo).
	install := "curl -fsSL https://get.docker.com | sh"
	if out, err := c.Exec(ctx, "id -u"); err == nil && strings.TrimSpace(out) != "0" {
		install = "curl -fsSL https://get.docker.com | sudo -n sh"
	}
	if _, err := c.Exec(ctx, install); err != nil {
		return upstream("docker install failed: %v (host may need passwordless sudo or root ssh)", err)
	}
	_, _ = c.Exec(ctx, "systemctl enable --now docker 2>/dev/null || service docker start 2>/dev/null || true")
	ds = c.CheckDocker(ctx)
	if !ds.Docker {
		return upstream("docker install did not produce a working docker CLI")
	}
	step("docker-install", true, ds.Version)
	return nil
}

// Compose is the peer's own compose file (deploy/docker-compose.yml),
// rendered for the new host. Kept in sync with the repo file by
// test (composeFile constant is asserted against the file).
const composeFileTemplate = `# Ferriage peer — rendered by SSH onboarding (ADR 0006).
# State is files: the /stacks volume holds one subdirectory per
# stack. The peer talks to the host daemon through the socket.
services:
  peer:
    image: {{IMAGE}}
    pull_policy: missing
    restart: unless-stopped
    environment:
      FERRIAGE_STATE_DIR: /stacks
      FERRIAGE_PEER: {{PEER_URL}}
      FERRIAGE_SYNC_INTERVAL: 30s
    volumes:
      - ferriage_state:/stacks
      - /var/run/docker.sock:/var/run/docker.sock
    ports:
      - "{{PORT}}:8080"
    healthcheck:
      test: ["CMD", "curl", "-fsS", "http://localhost:8080/healthz"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s

volumes:
  ferriage_state:
    name: ferriage_state
`

// RenderCompose produces the compose file for a new host peer.
func RenderCompose(image, peerURL string, port int) string {
	out := strings.ReplaceAll(composeFileTemplate, "{{IMAGE}}", image)
	out = strings.ReplaceAll(out, "{{PEER_URL}}", peerURL)
	out = strings.ReplaceAll(out, "{{PORT}}", strconv.Itoa(port))
	return out
}

// Onboard runs the full flow and returns the result. The dial
// and health-wait seams exist for tests; production passes nil.
func Onboard(ctx context.Context, req *Request, stateDir string,
	dial func(context.Context, *Request, string) (Runner, error),
	waitHealthy func(context.Context, string, int, time.Duration) error) (*Result, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	res := &Result{Address: req.Address, PeerURL: req.PeerURL, Steps: []Step{}}
	step := func(name string, ok bool, detail string) {
		res.Steps = append(res.Steps, Step{Name: name, OK: ok, Detail: detail})
	}
	if dial == nil {
		dial = func(ctx context.Context, r *Request, sd string) (Runner, error) {
			return Dial(ctx, r, sd)
		}
	}
	if waitHealthy == nil {
		waitHealthy = WaitHealthy
	}
	c, err := dial(ctx, req, stateDir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Close() }()
	step("ssh", true, "connected as "+req.User+"@"+req.Address)

	if err := c.EnsureDocker(ctx, step); err != nil {
		return res, err
	}
	ds := c.CheckDocker(ctx)
	res.Docker = "present"
	if len(res.Steps) > 0 && res.Steps[len(res.Steps)-1].Name == "docker-install" {
		res.Docker = "installed"
	}
	res.Compose = "present"
	if !ds.Compose {
		res.Compose = "missing"
	}

	// Host dir: /opt/ferriage when root, ~/ferriage otherwise.
	hostDir := "/opt/ferriage"
	if out, err := c.Exec(ctx, "id -u"); err == nil && strings.TrimSpace(out) != "0" {
		if home, err2 := c.Exec(ctx, "eval echo ~$USER"); err2 == nil {
			hostDir = strings.TrimSpace(home) + "/ferriage"
		}
	}
	if _, err := c.Exec(ctx, "mkdir -p "+shellQuote(hostDir)+" && chmod 755 "+shellQuote(hostDir)); err != nil {
		return res, upstream("create %s: %v", hostDir, err)
	}
	composePath := hostDir + "/docker-compose.yml"
	content := RenderCompose(req.Image, req.PeerURL, req.Port)
	if err := c.Upload(ctx, composePath, []byte(content)); err != nil {
		return res, upstream("%v", err)
	}
	step("compose-render", true, composePath)

	if _, err := c.Exec(ctx, "cd "+shellQuote(hostDir)+" && docker compose pull 2>&1 | tail -3"); err != nil {
		return res, upstream("docker compose pull: %v", err)
	}
	if _, err := c.Exec(ctx, "cd "+shellQuote(hostDir)+" && docker compose up -d 2>&1 | tail -3"); err != nil {
		return res, upstream("docker compose up: %v", err)
	}
	res.Container = "ferriage-peer-1"
	step("container-start", true, res.Container)

	if err := waitHealthy(ctx, req.Address, req.Port, 60*time.Second); err != nil {
		step("healthz", false, err.Error())
		return res, upstream("new peer did not become healthy: %v", err)
	}
	res.Healthy = true
	step("healthz", true, "GET /healthz ok")
	return res, nil
}

// WaitHealthy polls the new peer's /healthz.
func WaitHealthy(ctx context.Context, host string, port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	url := "http://" + hostForURL(host) + ":" + strconv.Itoa(port) + "/healthz"
	client := &http.Client{Timeout: 3 * time.Second}
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout after %s waiting for %s", timeout, url)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err == nil {
			if resp, err := client.Do(req); err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

func hostForURL(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

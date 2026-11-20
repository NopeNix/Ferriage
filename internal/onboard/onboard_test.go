package onboard

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"strings"
	"testing"
	"time"
)

// pkcs8 wraps an ed25519 seed as PKCS#8 DER (RFC 8410).
func pkcs8(priv ed25519.PrivateKey) []byte {
	der := []byte{0x30, 0x2e, 0x02, 0x01, 0x00, 0x30, 0x05, 0x06, 0x03, 0x2b, 0x65, 0x70, 0x04, 0x22, 0x04, 0x20}
	return append(der, priv.Seed()...)
}

func testKey(t *testing.T) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8(priv)}))
}

func TestValidate(t *testing.T) {
	key := testKey(t)
	req := &Request{Address: "192.168.1.10:22", PrivateKey: key}
	if err := req.Validate(); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
	if req.User != "root" || req.Port != 8080 {
		t.Fatalf("defaults not applied: %+v", req)
	}
	for _, tc := range []struct {
		name string
		req  *Request
	}{
		{"empty address", &Request{PrivateKey: key}},
		{"bad address", &Request{Address: "a b", PrivateKey: key}},
		{"no key", &Request{Address: "h"}},
		{"not pem", &Request{Address: "h", PrivateKey: "hello"}},
		{"bad user", &Request{Address: "h", PrivateKey: key, User: "root;rm -rf /"}},
		{"bad port", &Request{Address: "h", PrivateKey: key, Port: 70000}},
	} {
		if err := tc.req.Validate(); err == nil {
			t.Fatalf("%s: expected error", tc.name)
		}
	}
}

func TestRenderCompose(t *testing.T) {
	out := RenderCompose("nopenix/ferriage:v1", "http://10.0.0.1:8080", 8080)
	for _, want := range []string{"image: nopenix/ferriage:v1", "FERRIAGE_PEER: http://10.0.0.1:8080", `"8080:8080"`, "ferriage_state:/stacks", "/var/run/docker.sock"} {
		if !strings.Contains(out, want) {
			t.Fatalf("rendered compose missing %q", want)
		}
	}
	if strings.Contains(out, "{{") {
		t.Fatalf("unrendered template:\n%s", out)
	}
}

// fakeRunner is the scripted Runner for flow tests.
type fakeRunner struct {
	execQueue []string
	uploads   map[string]string
	check     DockerStatus
}

func (f *fakeRunner) Exec(ctx context.Context, cmd string) (string, error) {
	if len(f.execQueue) > 0 {
		out := f.execQueue[0]
		f.execQueue = f.execQueue[1:]
		return out, nil
	}
	return "", nil
}
func (f *fakeRunner) Upload(ctx context.Context, path string, content []byte) error {
	if f.uploads == nil {
		f.uploads = map[string]string{}
	}
	f.uploads[path] = string(content)
	return nil
}
func (f *fakeRunner) CheckDocker(ctx context.Context) DockerStatus {
	if f.check.Version == "" {
		return DockerStatus{Docker: true, Compose: true, Version: "Docker version 29.0.0"}
	}
	return f.check
}
func (f *fakeRunner) EnsureDocker(ctx context.Context, step func(string, bool, string)) error {
	step("docker-check", true, f.CheckDocker(ctx).Version)
	return nil
}
func (f *fakeRunner) Close() error { return nil }

func TestOnboardHappyPath(t *testing.T) {
	fc := &fakeRunner{execQueue: []string{"0\n", "", "", ""}}
	req := &Request{Address: "192.168.1.10", PrivateKey: testKey(t), PeerURL: "http://10.0.0.1:8080"}
	dial := func(ctx context.Context, r *Request, sd string) (Runner, error) { return fc, nil }
	wait := func(ctx context.Context, host string, port int, timeout time.Duration) error { return nil }
	res, err := Onboard(context.Background(), req, t.TempDir(), dial, wait)
	if err != nil {
		t.Fatalf("onboard: %v", err)
	}
	if !res.Healthy || res.Docker != "present" || res.Compose != "present" {
		t.Fatalf("bad result: %+v", res)
	}
	wantSteps := []string{"ssh", "docker-check", "compose-render", "container-start", "healthz"}
	if len(res.Steps) != len(wantSteps) {
		t.Fatalf("steps = %v", res.Steps)
	}
	for i, name := range wantSteps {
		if res.Steps[i].Name != name || !res.Steps[i].OK {
			t.Fatalf("step %d = %+v, want %s ok", i, res.Steps[i], name)
		}
	}
	if got := fc.uploads["/opt/ferriage/docker-compose.yml"]; !strings.Contains(got, "FERRIAGE_PEER: http://10.0.0.1:8080") {
		t.Fatalf("uploaded compose wrong:\n%s", got)
	}
}

func TestOnboardBadRequest(t *testing.T) {
	_, err := Onboard(context.Background(), &Request{}, t.TempDir(), nil, nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if e, ok := err.(*Error); !ok || e.Code != "bad_request" {
		t.Fatalf("want bad_request, got %v", err)
	}
}

func TestOnboardHealthGateFailure(t *testing.T) {
	fc := &fakeRunner{execQueue: []string{"0\n", "", "", ""}}
	req := &Request{Address: "192.168.1.10", PrivateKey: testKey(t)}
	dial := func(ctx context.Context, r *Request, sd string) (Runner, error) { return fc, nil }
	wait := func(ctx context.Context, host string, port int, timeout time.Duration) error {
		return errTimeout
	}
	_, err := Onboard(context.Background(), req, t.TempDir(), dial, wait)
	if err == nil || !strings.Contains(err.Error(), "did not become healthy") {
		t.Fatalf("want health-gate failure, got %v", err)
	}
}

var errTimeout = &timeoutErr{}

type timeoutErr struct{}

func (*timeoutErr) Error() string { return "timeout" }

func TestShellQuote(t *testing.T) {
	if got := shellQuote("/opt/ferriage"); got != "'/opt/ferriage'" {
		t.Fatalf("shellQuote = %s", got)
	}
	if got := shellQuote("a'b"); got != "'a'\\''b'" {
		t.Fatalf("shellQuote quote = %s", got)
	}
}

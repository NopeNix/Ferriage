package audit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAppendAndQueryNewestFirst(t *testing.T) {
	dir := t.TempDir()
	s := New(dir, 0, false)
	if err := s.Append(Entry{Actor: "alice", Action: "deploy", Target: "demo", Status: 200, Ts: "2026-11-20T10:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Append(Entry{Actor: "bob", Action: "exec", Target: "demo/app", Status: 502, Ts: "2026-11-20T11:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	got := s.Query(Filter{})
	if len(got) != 2 {
		t.Fatalf("query = %d entries, want 2", len(got))
	}
	if got[0].Actor != "bob" || got[0].Action != "exec" || got[0].Status != 502 {
		t.Errorf("newest first = %+v, want bob/exec/502", got[0])
	}
	if got[1].Actor != "alice" {
		t.Errorf("second = %+v, want alice", got[1])
	}
	// The file lives under .ferriage/, never in the synced tree.
	data, err := os.ReadFile(filepath.Join(dir, ".ferriage", "audit.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("file lines = %d, want 2", len(lines))
	}
	var first map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatal(err)
	}
	if first["actor"] != "alice" {
		t.Errorf("first line actor = %v, want alice", first["actor"])
	}
}

func TestQueryFilters(t *testing.T) {
	dir := t.TempDir()
	s := New(dir, 0, false)
	_ = s.Append(Entry{Actor: "alice", Action: "deploy", Target: "a", Status: 200, Ts: "2026-11-20T10:00:00Z"})
	_ = s.Append(Entry{Actor: "bob", Action: "exec", Target: "a/app", Status: 200, Ts: "2026-11-20T11:00:00Z"})
	_ = s.Append(Entry{Actor: "alice", Action: "exec", Target: "a/app", Status: 200, Ts: "2026-11-20T12:00:00Z"})

	if got := s.Query(Filter{Actor: "alice"}); len(got) != 2 {
		t.Errorf("actor=alice = %d, want 2", len(got))
	}
	if got := s.Query(Filter{Action: "exec"}); len(got) != 2 {
		t.Errorf("action=exec = %d, want 2", len(got))
	}
	since, _ := time.Parse(time.RFC3339, "2026-11-20T11:30:00Z")
	if got := s.Query(Filter{Since: since}); len(got) != 1 || got[0].Ts != "2026-11-20T12:00:00Z" {
		t.Errorf("since filter = %+v, want just the 12:00 entry", got)
	}
	if got := s.Query(Filter{Limit: 1}); len(got) != 1 {
		t.Errorf("limit=1 = %d, want 1", len(got))
	}
	if got := s.Query(Filter{Actor: "nobody"}); len(got) != 0 {
		t.Errorf("unknown actor = %d, want 0", len(got))
	}
}

func TestAppendDefaultsActorAndTs(t *testing.T) {
	dir := t.TempDir()
	s := New(dir, 0, false)
	if err := s.Append(Entry{Action: "deploy", Status: 200}); err != nil {
		t.Fatal(err)
	}
	got := s.Query(Filter{})
	if len(got) != 1 {
		t.Fatalf("query = %d, want 1", len(got))
	}
	if got[0].Actor != "unknown" {
		t.Errorf("actor = %q, want unknown", got[0].Actor)
	}
	if got[0].Ts == "" {
		t.Error("ts should be defaulted")
	}
}

func TestAppendDisabledIsNoop(t *testing.T) {
	dir := t.TempDir()
	s := New(dir, 0, true)
	if err := s.Append(Entry{Actor: "alice", Action: "deploy", Status: 200}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".ferriage", "audit.jsonl")); !os.IsNotExist(err) {
		t.Error("disabled store should not create the file")
	}
	if got := s.Query(Filter{}); len(got) != 0 {
		t.Errorf("disabled query = %d, want 0", len(got))
	}
	var nilStore *Store
	_ = nilStore.Append(Entry{Action: "deploy"})
	if got := nilStore.Query(Filter{}); len(got) != 0 {
		t.Errorf("nil store query = %d, want 0", len(got))
	}
}

func TestPruneKeepsNewest(t *testing.T) {
	dir := t.TempDir()
	s := New(dir, 3, false)
	for i := 0; i < 10; i++ {
		ts := time.Date(2026, 11, 20, 10, i, 0, 0, time.UTC).Format(time.RFC3339)
		if err := s.Append(Entry{Actor: "alice", Action: "deploy", Status: 200, Ts: ts}); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, ".ferriage", "audit.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("after prune lines = %d, want 3", len(lines))
	}
	got := s.Query(Filter{})
	if len(got) != 3 || got[0].Ts != "2026-11-20T10:09:00Z" {
		t.Errorf("after prune newest = %+v, want 10:09 first", got)
	}
}

func TestMiddlewareRecordsMutatingOnly(t *testing.T) {
	dir := t.TempDir()
	s := New(dir, 0, false)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /stacks/{name}/deploy", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	})
	mux.HandleFunc("GET /stacks", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"stacks":[]}`))
	})
	mux.HandleFunc("PUT /stacks/{name}/compose", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"name":"demo"}`))
	})
	h := Middleware(s, mux)

	// Mutating + actor header → recorded with status.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/stacks/demo/deploy", nil)
	req.Header.Set(ActorHeader, "alice@example.com")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("deploy status = %d, want 502 (middleware must not eat the response)", rec.Code)
	}

	// Read → not recorded.
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/stacks", nil))

	// Compose PUT → recorded as compose.save.
	rec3 := httptest.NewRecorder()
	h.ServeHTTP(rec3, httptest.NewRequest(http.MethodPut, "/stacks/demo/compose", nil))

	got := s.Query(Filter{})
	if len(got) != 2 {
		t.Fatalf("recorded = %d, want 2 (deploy + compose.save)", len(got))
	}
	// Both entries share the same second; order between them
	// is not guaranteed — check the set, not the sequence.
	byAction := map[string]struct {
		actor, target string
		status        int
	}{}
	for _, e := range got {
		byAction[e.Action] = struct {
			actor, target string
			status        int
		}{e.Actor, e.Target, e.Status}
	}
	if d := byAction["deploy"]; d.actor != "alice@example.com" || d.target != "demo" || d.status != http.StatusBadGateway {
		t.Errorf("deploy entry = %+v", d)
	}
	if c := byAction["compose.save"]; c.actor != "unknown" || c.target != "demo" || c.status != http.StatusOK {
		t.Errorf("compose entry = %+v", c)
	}
}

func TestClassifyUnknownMutatingRecorded(t *testing.T) {
	s := New(t.TempDir(), 0, false)
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /some/future/route", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	rec := httptest.NewRecorder()
	Middleware(s, mux).ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/some/future/route", nil))
	got := s.Query(Filter{})
	if len(got) != 1 || got[0].Action != "/some/future/route" {
		t.Errorf("unknown mutating route = %+v, want one generic entry", got)
	}
}

func TestHandlerFiltersAndValidation(t *testing.T) {
	dir := t.TempDir()
	s := New(dir, 0, false)
	_ = s.Append(Entry{Actor: "alice", Action: "deploy", Target: "demo", Status: 200})
	mux := http.NewServeMux()
	Register(mux, s)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/audit?actor=alice", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /audit = %d", rec.Code)
	}
	var body struct {
		Entries []map[string]any `json:"entries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(body.Entries))
	}

	for _, path := range []string{"/audit?limit=0", "/audit?limit=501", "/audit?limit=abc", "/audit?since=not-a-time"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("GET %s = %d, want 400", path, rec.Code)
		}
	}

	// No write/delete surface exists by design.
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(m, "/audit", nil))
		if rec.Code == http.StatusOK {
			t.Errorf("%s /audit should not be served", m)
		}
	}
}

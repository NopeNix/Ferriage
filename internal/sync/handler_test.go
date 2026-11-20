package sync

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHandlers(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "web/compose.yaml", []byte("services: {}\n"))
	s := New(root, "peer-a")
	h := NewHandler(s, "peer-a")

	// /peer
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/peer", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/peer status = %d", rec.Code)
	}
	var peer map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&peer); err != nil || peer["peer_id"] != "peer-a" {
		t.Fatalf("/peer body = %v, %v", peer, err)
	}

	// /sync/manifest
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/sync/manifest", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/sync/manifest status = %d", rec.Code)
	}
	var m Manifest
	if err := json.NewDecoder(rec.Body).Decode(&m); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if len(m.Entries) != 1 || m.Entries[0].Path != "web/compose.yaml" {
		t.Fatalf("manifest = %+v", m.Entries)
	}

	// /sync/blob happy path
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/sync/blob?path=web/compose.yaml", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "services: {}\n" {
		t.Fatalf("blob = %d %q", rec.Code, rec.Body.String())
	}

	// path traversal → 400
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/sync/blob?path=../secret", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("traversal status = %d, want 400", rec.Code)
	}

	// missing blob → 404
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/sync/blob?path=nope/x", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing status = %d, want 404", rec.Code)
	}

	// .ferriage is never served
	writeFile(t, root, ".ferriage/identity.json", []byte(`{"x":1}`))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/sync/blob?path=.ferriage/identity.json", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf(".ferriage status = %d, want 400", rec.Code)
	}

	// wrong method → 405
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/sync/manifest", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST manifest status = %d, want 405", rec.Code)
	}

	// a symlink escaping the tree is never served
	if err := os.Symlink("/etc/hostname", filepath.Join(root, "escape")); err == nil {
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/sync/blob?path=escape", nil))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("symlink escape status = %d, want 400", rec.Code)
		}
	}
}

package onboard

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerBadBody(t *testing.T) {
	h := &Handler{StateDir: t.TempDir()}
	req := httptest.NewRequest(http.MethodPost, "/hosts/onboard", strings.NewReader("{not json"))
	rec := httptest.NewRecorder()
	h.onboard(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestHandlerValidation(t *testing.T) {
	h := &Handler{StateDir: t.TempDir()}
	req := httptest.NewRequest(http.MethodPost, "/hosts/onboard", strings.NewReader(`{"address":""}`))
	rec := httptest.NewRecorder()
	h.onboard(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "address") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

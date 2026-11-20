package stacks

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/NopeNix/Ferriage/internal/adopt"
)

func TestReturnsStacks(t *testing.T) {
	h := NewHandlerWithLister(listerFunc(func(context.Context) ([]adopt.Stack, error) {
		return []adopt.Stack{{Name: "demo", ContainerCount: 1, RunningCount: 1}}, nil
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/stacks", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got Response
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Stacks) != 1 || got.Stacks[0].Name != "demo" {
		t.Errorf("stacks = %+v", got.Stacks)
	}
}

func TestErrorIs502(t *testing.T) {
	h := NewHandlerWithLister(listerFunc(func(context.Context) ([]adopt.Stack, error) {
		return nil, errors.New("daemon down")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/stacks", nil))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
}

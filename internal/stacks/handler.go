// Package stacks serves the adopted-stack list on the peer API.
package stacks

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/NopeNix/Ferriage/internal/adopt"
)

// Lister supplies the adopted stacks for a request.
type Lister interface {
	List(context.Context) ([]adopt.Stack, error)
}

type listerFunc func(context.Context) ([]adopt.Stack, error)

func (f listerFunc) List(ctx context.Context) ([]adopt.Stack, error) { return f(ctx) }

// Response is the body of GET /stacks.
type Response struct {
	Stacks []adopt.Stack `json:"stacks"`
}

// NewHandler returns the stacks endpoint handler. The docker client is
// created per request: the peer is low-traffic, and this keeps daemon
// connectivity failures visible as 502s rather than caching a broken
// client for the life of the process.
func NewHandler() http.Handler {
	return NewHandlerWithLister(defaultLister{})
}

type defaultLister struct{}

func (defaultLister) List(ctx context.Context) ([]adopt.Stack, error) {
	c, err := adopt.NewClient()
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Close() }()
	return adopt.List(ctx, c)
}

// NewHandlerWithLister allows tests to inject a fake lister.
func NewHandlerWithLister(l Lister) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stacks, err := l.List(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Response{Stacks: stacks})
	})
}

func writeError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadGateway)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

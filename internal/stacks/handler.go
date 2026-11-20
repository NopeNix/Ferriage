// Package stacks serves the adopted-stack list on the peer API.
package stacks

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/NopeNix/Ferriage/internal/adopt"
	"github.com/NopeNix/Ferriage/internal/api"
)

// Lister supplies the adopted stacks for a request.
type Lister interface {
	List(context.Context) ([]adopt.Stack, error)
}

type listerFunc func(context.Context) ([]adopt.Stack, error)

func (f listerFunc) List(ctx context.Context) ([]adopt.Stack, error) { return f(ctx) }

// Response is the body of GET /stacks.
type Response = api.StacksResponse

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
		out := make([]api.Stack, 0, len(stacks))
		for _, s := range stacks {
			out = append(out, toAPIStack(s))
		}
		_ = json.NewEncoder(w).Encode(Response{Stacks: out})
	})
}

func writeError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadGateway)
	_ = json.NewEncoder(w).Encode(api.ErrorResponse{Error: err.Error()})
}

// toAPIStack maps the adopt package's wire shape onto the typed
// API contract (internal/api). The two are kept structurally
// identical by the drift test in internal/api.
func toAPIStack(s adopt.Stack) api.Stack {
	svcs := make([]api.Service, 0, len(s.Services))
	for _, svc := range s.Services {
		svcs = append(svcs, api.Service{
			Name:      svc.Name,
			Container: svc.Container,
			Image:     svc.Image,
			State:     svc.State,
			Running:   svc.Running,
		})
	}
	return api.Stack{
		Name:           s.Name,
		ComposeFile:    s.ComposeFile,
		WorkingDir:     s.WorkingDir,
		Services:       svcs,
		RunningCount:   s.RunningCount,
		ContainerCount: s.ContainerCount,
	}
}

// Command peer is the Ferriage peer binary: every node runs it.
// Phase 1.1 scope: start, serve /healthz. Mesh/state arrive in later steps.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/NopeNix/Ferriage/internal/health"
	"github.com/NopeNix/Ferriage/internal/stacks"
	"github.com/NopeNix/Ferriage/internal/state"
)

func main() {
	addr := os.Getenv("FERRIAGE_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	stateDir := os.Getenv("FERRIAGE_STATE_DIR")
	if stateDir == "" {
		stateDir = "./stacks"
	}
	if _, err := state.New(stateDir); err != nil {
		log.Fatalf("state dir: %v", err)
	}
	log.Printf("ferriage peer state dir: %s", stateDir)

	mux := http.NewServeMux()
	mux.Handle("/healthz", health.NewHandler())
	mux.Handle("/stacks", stacks.NewHandler())

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	go func() {
		log.Printf("ferriage peer listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("server error: %v", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
		os.Exit(1)
	}
	log.Println("ferriage peer stopped")
}

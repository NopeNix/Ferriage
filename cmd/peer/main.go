// Command peer is the Ferriage peer binary: every node runs it.
// Phase 1.1: /healthz. Phase 1.2: /stacks (adopt by labels).
// Phase 1.3: mesh sync of the state dir (ADR 0001).
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/NopeNix/Ferriage/internal/health"
	"github.com/NopeNix/Ferriage/internal/identity"
	"github.com/NopeNix/Ferriage/internal/stacks"
	"github.com/NopeNix/Ferriage/internal/state"
	"github.com/NopeNix/Ferriage/internal/sync"
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

	idn, err := identity.LoadOrCreate(stateDir)
	if err != nil {
		log.Fatalf("identity: %v", err)
	}
	log.Printf("ferriage peer id: %s...", idn.ID()[:16])

	store := sync.New(stateDir, idn.ID())
	ledger, err := sync.LoadLedger(stateDir)
	if err != nil {
		log.Fatalf("sync ledger: %v", err)
	}

	peers := splitPeers(os.Getenv("FERRIAGE_PEER"))
	syncInterval := 30 * time.Second
	if v := os.Getenv("FERRIAGE_SYNC_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			syncInterval = d
		}
	}
	if len(peers) > 0 {
		log.Printf("ferriage sync: pulling from %v every %s", peers, syncInterval)
	}

	mux := http.NewServeMux()
	mux.Handle("/healthz", health.NewHandler())
	mux.Handle("/stacks", stacks.NewHandler())
	mux.Handle("/", sync.NewHandler(store, idn.ID()))

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if len(peers) > 0 {
		fetch := &sync.HTTPFetcher{Client: &http.Client{Timeout: 30 * time.Second}}
		go sync.Loop(ctx, store, ledger, fetch, peers, syncInterval)
	}

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

func splitPeers(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

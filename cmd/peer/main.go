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
	"strconv"
	"strings"
	"time"

	"github.com/NopeNix/Ferriage/internal/api"
	"github.com/NopeNix/Ferriage/internal/audit"
	"github.com/NopeNix/Ferriage/internal/compose"
	"github.com/NopeNix/Ferriage/internal/console"
	"github.com/NopeNix/Ferriage/internal/deploy"
	"github.com/NopeNix/Ferriage/internal/exec"
	"github.com/NopeNix/Ferriage/internal/health"
	"github.com/NopeNix/Ferriage/internal/identity"
	"github.com/NopeNix/Ferriage/internal/logs"
	"github.com/NopeNix/Ferriage/internal/notify"
	"github.com/NopeNix/Ferriage/internal/onboard"
	"github.com/NopeNix/Ferriage/internal/snapshot"
	"github.com/NopeNix/Ferriage/internal/stacks"
	"github.com/NopeNix/Ferriage/internal/state"
	"github.com/NopeNix/Ferriage/internal/stats"
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
	// The URL new hosts pull mesh state from when they are
	// onboarded through this peer (ADR 0001 pull, ADR 0006).
	selfURL := os.Getenv("FERRIAGE_SELF_URL")
	if selfURL == "" {
		selfURL = "http://" + addr
	}
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
	mux.Handle("/openapi.json", api.Handler())
	mux.Handle("/openapi.yaml", api.Handler())
	mux.Handle("/stacks", stacks.NewHandler())
	mux.Handle("/", sync.NewHandler(store, idn.ID()))
	snapshot.Register(mux, stateDir, idn.ID())
	// Notifications (ADR 0007, option A): Shoutrrr URLs from
	// FERRIAGE_NOTIFY_URLS only; a nil/disabled notifier is a
	// no-op, so handlers stay unconditional.
	notifier, _ := notify.New(notify.URLsFromEnv(), idn.ID)
	if notifier.Enabled() {
		log.Printf("ferriage notify: %d channel(s) enabled (FERRIAGE_NOTIFY_URLS)", notifier.Channels())
	}
	deploy.Register(mux, stateDir, notifier)
	logs.Register(mux)
	stats.Register(mux)
	exec.Register(mux)
	compose.Register(mux, stateDir)
	console.Register(mux)
	onboard.Register(mux, stateDir, selfURL, notifier)
	notify.Register(mux, notifier)
	// Audit log (ADR 0008): append-only JSONL under
	// .ferriage/, recorded by middleware around the whole
	// mux so actor/status are captured uniformly. Reads and
	// mesh machinery are never recorded.
	auditStore := audit.New(stateDir, auditMaxLines(), os.Getenv("FERRIAGE_AUDIT_DISABLE") == "1")
	audit.Register(mux, auditStore)
	if !auditStore.Disabled() {
		log.Printf("ferriage audit: logging to .ferriage/audit.jsonl (max %d lines)", auditMaxLines())
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           audit.Middleware(auditStore, mux),
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

// auditMaxLines reads FERRIAGE_AUDIT_MAX_LINES (default
// audit.DefaultMaxLines). An unparsable or non-positive value
// falls back to the default, so recovery never disables the log.
func auditMaxLines() int {
	v := os.Getenv("FERRIAGE_AUDIT_MAX_LINES")
	if v == "" {
		return audit.DefaultMaxLines
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return audit.DefaultMaxLines
	}
	return n
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

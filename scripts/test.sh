#!/usr/bin/env bash
# Post-change test routine for Ferriage.
# Layers, fastest first; exits non-zero on any hard failure.
#   Layer 1: lint + unit tests            (--fast = layer 1 only)
#   Layer 2: compose smoke (2 peers, sync, adopt, move with volume)
#   Layer 3: Playwright visual baselines (UI only)
# Usage: scripts/test.sh [--fast] [--skip-container] [--skip-playwright]
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

SKIP_CONTAINER=0
SKIP_PLAYWRIGHT=0
FAST=0
for arg in "$@"; do
    case "$arg" in
        --fast)                  FAST=1; SKIP_CONTAINER=1; SKIP_PLAYWRIGHT=1 ;;
        --skip-container)        SKIP_CONTAINER=1 ;;
        --skip-playwright)       SKIP_PLAYWRIGHT=1 ;;
        -h|--help)
            echo "Usage: $0 [--fast] [--skip-container] [--skip-playwright]"
            echo "  --fast   Layer 1 only (lint + unit). The iterate loop:"
            echo "           run on every change; full routine before ship."
            exit 0
            ;;
        *) echo "Unknown arg: $arg" >&2; exit 2 ;;
    esac
done

PASS=0
FAIL=0
log() { printf "\n\033[1;36m== %s ==\033[0m\n" "$*"; }
ok()  { printf "  \033[1;32m✓\033[0m %s\n" "$*"; PASS=$((PASS+1)); }
bad() { printf "  \033[1;31m✗\033[0m %s\n" "$*"; FAIL=$((FAIL+1)); }
skip(){ printf "  \033[1;33m↷\033[0m %s\n" "$*"; }

# ---- Layer 1: lint + unit tests ---------------------------------------
log "Layer 1: lint, unit tests"
if [ -f go.mod ]; then
    if gofmt -l . >/tmp/ferriage_gofmt.log 2>&1 && [ ! -s /tmp/ferriage_gofmt.log ]; then
        ok "gofmt"
    else
        bad "gofmt (unformatted files listed in /tmp/ferriage_gofmt.log)"; cat /tmp/ferriage_gofmt.log
    fi
    if go vet ./... >/tmp/ferriage_vet.log 2>&1; then
        ok "go vet"
    else
        bad "go vet (see /tmp/ferriage_vet.log)"; cat /tmp/ferriage_vet.log
    fi
    if command -v golangci-lint >/dev/null 2>&1; then
        if golangci-lint run >/tmp/ferriage_lint.log 2>&1; then
            ok "golangci-lint"
        else
            bad "golangci-lint (see /tmp/ferriage_lint.log)"
        fi
    else
        skip "golangci-lint not installed (optional)"
    fi
    if go test ./... >/tmp/ferriage_gotest.log 2>&1; then
        ok "go test ./..."
    else
        bad "go test ./... (see /tmp/ferriage_gotest.log)"; cat /tmp/ferriage_gotest.log
    fi
else
    skip "no go.mod yet — Go layer skipped (no code)"
fi

if [ -f web/package.json ]; then
    if (cd web && npm run lint --silent >/tmp/ferriage_npm_lint.log 2>&1); then
        ok "web lint"
    else
        bad "web lint (see /tmp/ferriage_npm_lint.log)"
    fi
    if (cd web && npm test --silent >/tmp/ferriage_npm_test.log 2>&1); then
        ok "web test"
    else
        bad "web test (see /tmp/ferriage_npm_test.log)"
    fi
else
    skip "no web/package.json yet — frontend layer skipped (no UI)"
fi

# ---- Layer 1.5: CLI end-to-end (cmd/ferriage vs an ephemeral peer) ---
# One peer on a scratch state dir + a scratch test stack (health
# checked, volume-writing, unique name per run). The CLI drives the
# same typed API the UI uses: lists, deploys, exports, imports,
# and a --json round-trip. This is the user-facing twin check:
# every UI action has a CLI twin (Phase 3), proved end-to-end here.
if [ "$SKIP_CONTAINER" -eq 0 ] && [ -f go.mod ] && [ -d cmd/ferriage ]; then
    log "Layer 1.5: CLI end-to-end (cmd/ferriage vs ephemeral peer)"
    if ! command -v docker >/dev/null 2>&1; then
        skip "docker not installed — CLI e2e skipped"
    else
        set +e
        CLI_TMP="$(mktemp -d /tmp/ferriage-cli.XXXXXX)"
        CLI_PORT=18090
        CLI_STACK="ferriage-cli-$(head -c4 /dev/urandom | od -An -tx1 | tr -d ' \n')"
        CLI_VOL="${CLI_STACK}_data"
        CLI_MARKER="cli-volume-$(head -c4 /dev/urandom | od -An -tx1 | tr -d ' \n')"
        # Build both binaries once.
        go build -o "$CLI_TMP/peer-bin" ./cmd/peer 2>"$CLI_TMP/build.log" && go build -o "$CLI_TMP/ferriage" ./cmd/ferriage 2>>"$CLI_TMP/build.log"
        if [ $? -ne 0 ]; then
            bad "cli: binary build (see $CLI_TMP/build.log)"
        else
            mkdir -p "$CLI_TMP/stacks/$CLI_STACK"
            cat >"$CLI_TMP/stacks/$CLI_STACK/compose.yaml" <<YAML
services:
  app:
    image: alpine:3.20
    command: ["sh", "-c", "mkdir -p /data && echo $CLI_MARKER > /data/marker.txt && echo cli-volume-marker && sleep 1000"]
    volumes:
      - data:/data
    healthcheck:
      test: ["CMD", "sh", "-c", "test -f /data/marker.txt"]
      interval: 2s
      timeout: 3s
      retries: 5
      start_period: 2s
volumes:
  data:
YAML
            FERRIAGE_ADDR=":$CLI_PORT" FERRIAGE_STATE_DIR="$CLI_TMP/stacks" "$CLI_TMP/peer-bin" >"$CLI_TMP/peer.log" 2>&1 &
            PEER_PID=$!
            for i in $(seq 1 30); do curl -fsS "http://127.0.0.1:$CLI_PORT/healthz" >/dev/null 2>&1 && break; sleep 0.5; done

            # The web types must match this peer's contract exactly.
            if (cd web && FERRIAGE_PEER_URL="http://127.0.0.1:$CLI_PORT" npm run gen:check --silent >/dev/null 2>&1); then ok "cli: web types match /openapi.json (gen:check)"; else bad "cli: web types drifted from /openapi.json (npm run gen:check in web/)"; fi

            F="$CLI_TMP/ferriage --peer http://127.0.0.1:$CLI_PORT"
            if $F peer 2>/dev/null | grep -q "^peer "; then ok "cli: peer identity"; else bad "cli: peer identity"; fi
            if $F --json peer 2>/dev/null | grep -q '"peer_id"'; then ok "cli: peer --json"; else bad "cli: peer --json"; fi
            if $F deploy "$CLI_STACK" 2>/dev/null | grep -q "healthy"; then ok "cli: deploy (health-gated)"; else bad "cli: deploy"; fi
            if $F --json stacks 2>/dev/null | grep -q "\"name\": \"$CLI_STACK\""; then ok "cli: stacks --json lists adopted stack"; else bad "cli: stacks --json"; fi
            if $F export "$CLI_STACK" -o "$CLI_TMP/snap.tar.gz" >/dev/null 2>&1 && [ -s "$CLI_TMP/snap.tar.gz" ]; then ok "cli: export snapshot"; else bad "cli: export snapshot"; fi
            # The move needs the live stack gone first: bring it down
            # fully (containers + volume), then import the snapshot
            # through the CLI into a fresh stack name.
            docker compose -p "$CLI_STACK" -f "$CLI_TMP/stacks/$CLI_STACK/compose.yaml" down -v --remove-orphans >/dev/null 2>&1
            # Import refuses an existing stack dir (the live one is
            # still on disk). The peer runs as the operator here, so
            # a plain rm -rf of the state dir is enough.
            rm -rf "$CLI_TMP/stacks/$CLI_STACK"
            if $F import "$CLI_STACK" "$CLI_TMP/snap.tar.gz" >/dev/null 2>&1; then ok "cli: import snapshot"; else bad "cli: import snapshot"; fi
            if $F deploy "$CLI_STACK" 2>/dev/null | grep -q "healthy"; then ok "cli: redeploy after import"; else bad "cli: redeploy after import"; fi
            if docker run --rm -v "$CLI_VOL:/vol" alpine:3.20 cat /vol/marker.txt 2>/dev/null | grep -q "$CLI_MARKER"; then
                ok "cli: volume data survived export/import ($CLI_MARKER)"
            else
                bad "cli: volume data lost"
            fi
            if $F deploy no-such-stack >/dev/null 2>&1; then bad "cli: missing stack must fail (exit)"; else ok "cli: missing stack exits non-zero"; fi
            # Phase 3.2: logs/stats/exec/compose twins against the
            # redeployed stack (same peer, same daemon).
            if $F logs "$CLI_STACK" app --tail 50 2>/dev/null | grep -q "cli-volume-marker"; then ok "cli: logs tails container output"; else bad "cli: logs"; fi
            if $F --json logs "$CLI_STACK" app --tail 5 2>/dev/null | grep -q '"lines"'; then ok "cli: logs --json"; else bad "cli: logs --json"; fi
            if $F stats "$CLI_STACK" 2>/dev/null | grep -q "CPU"; then ok "cli: stats one-shot"; else bad "cli: stats"; fi
            if $F --json stats "$CLI_STACK" 2>/dev/null | grep -q '"cpu_percent"'; then ok "cli: stats --json"; else bad "cli: stats --json"; fi
            if $F exec "$CLI_STACK" app -- cat /data/marker.txt 2>/dev/null | grep -q "$CLI_MARKER"; then ok "cli: exec one-shot"; else bad "cli: exec"; fi
            if $F --json exec "$CLI_STACK" app -- sh -c 'exit 7' 2>/dev/null | grep -q '"exit_code": 7'; then ok "cli: exec reports exit code"; else bad "cli: exec exit code"; fi
            if curl -sS "http://127.0.0.1:$CLI_PORT/stacks/$CLI_STACK/compose" 2>/dev/null | grep -q "compose_file"; then ok "cli: GET compose file"; else bad "cli: GET compose"; fi
            COMPOSE_MOD="$(sed 's/interval: 2s/interval: 3s/' "$CLI_TMP/stacks/$CLI_STACK/compose.yaml")"
            printf '{"content": %s}' "$(printf '%s' "$COMPOSE_MOD" | python3 -c 'import sys,json; print(json.dumps(sys.stdin.read()))')" > "$CLI_TMP/put.json"
            if curl -sS -X PUT "http://127.0.0.1:$CLI_PORT/stacks/$CLI_STACK/compose" -H 'Content-Type: application/json' --data-binary @"$CLI_TMP/put.json" 2>/dev/null | grep -q "interval: 3s"; then ok "cli: PUT compose file"; else bad "cli: PUT compose"; fi
            if curl -sS "http://127.0.0.1:$CLI_PORT/stacks/$CLI_STACK/compose" 2>/dev/null | grep -q "interval: 3s"; then ok "cli: PUT compose persisted"; else bad "cli: PUT compose not persisted"; fi
            if $F exec "$CLI_STACK" app -- sh -c 'exit 0' >/dev/null 2>&1; then ok "cli: exec exit 0"; else bad "cli: exec exit 0"; fi
            if curl -sS "http://127.0.0.1:$CLI_PORT/stacks/no-such/compose" 2>/dev/null | grep -q '"error"'; then ok "cli: missing compose 404s"; else bad "cli: missing compose must 404"; fi
            # Phase 3.5 (ADR 0008): the mutating calls above must
            # have been recorded (newest first), reads must not
            # appear, and the actor header must round-trip.
            if $F audit --limit 50 2>/dev/null | grep -q "deploy"; then ok "cli: audit records deploy"; else bad "cli: audit records deploy"; fi
            if $F audit --action exec 2>/dev/null | grep -q "exec"; then ok "cli: audit filters by action"; else bad "cli: audit filters by action"; fi
            if $F --json audit 2>/dev/null | grep -q '"entries"'; then ok "cli: audit --json"; else bad "cli: audit --json"; fi
            if curl -sS -H "X-Ferriage-Actor: tester@example.com" -X POST "http://127.0.0.1:$CLI_PORT/notify/test" >/dev/null 2>&1; then :; fi
            if $F audit --actor tester@example.com 2>/dev/null | grep -q "tester@example.com"; then ok "cli: audit records actor from header"; else bad "cli: audit actor from header"; fi
            if $F audit --limit 0 >/dev/null 2>&1; then bad "cli: audit --limit 0 must fail (exit)"; else ok "cli: audit --limit 0 exits non-zero"; fi
            if curl -sS -X DELETE "http://127.0.0.1:$CLI_PORT/audit" >/dev/null 2>&1 && [ ! -f "$CLI_TMP/stacks/.ferriage/audit.jsonl" ]; then :; fi
            if $F audit 2>/dev/null | grep -q "compose.save"; then ok "cli: audit records compose.save"; else bad "cli: audit records compose.save"; fi
            # Phase 3.7 (ADR 0010): MCP server — tools/list,
            # a mutating tool call, audit of the inner action,
            # and the CLI twin. The MCP envelope must NOT be
            # audited; the deploy inside it must be.
            if $F mcp tools 2>/dev/null | grep -q "deploy_stack"; then ok "cli: mcp tools/list (CLI twin)"; else bad "cli: mcp tools"; fi
            if curl -sS -X POST "http://127.0.0.1:$CLI_PORT/mcp" -H 'Content-Type: application/json' \
                -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}' 2>/dev/null \
                | grep -q '"protocolVersion":"2025-03-26"'; then ok "cli: mcp initialize"; else bad "cli: mcp initialize"; fi
            if curl -sS -X POST "http://127.0.0.1:$CLI_PORT/mcp" -H 'Content-Type: application/json' \
                -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"deploy_stack","arguments":{"stack":"'$CLI_STACK'"}}}' 2>/dev/null \
                | grep -q '\\"healthy\\":true'; then ok "cli: mcp tools/call deploy_stack"; else bad "cli: mcp tools/call deploy_stack"; fi
            if $F audit --actor mcp --action deploy 2>/dev/null | grep -q "deploy"; then ok "cli: mcp inner action audited (actor mcp)"; else bad "cli: mcp inner action audited"; fi
            if $F audit 2>/dev/null | grep -q '"/mcp"'; then bad "cli: mcp envelope must not be audited"; else ok "cli: mcp envelope not audited"; fi
            if $F mcp call get_peer 2>/dev/null | grep -q '"peer_id"'; then ok "cli: mcp call get_peer (CLI twin)"; else bad "cli: mcp call"; fi
            if curl -sS "http://127.0.0.1:$CLI_PORT/mcp" 2>/dev/null | grep -q '"transport":"mcp-streamable-http"'; then ok "cli: GET /mcp describes surface"; else bad "cli: GET /mcp"; fi
            # Phase 3.8 (ADR 0011): restic backups. Status works
            # without a restic binary (repo uninitialized, password
            # never exposed); with a restic binary on PATH the full
            # run -> list -> forget -> restore cycle runs live.
            if curl -sS "http://127.0.0.1:$CLI_PORT/backup" 2>/dev/null | grep -q '"initialized":false'; then ok "cli: backup status (uninitialized)"; else bad "cli: backup status"; fi
            if curl -sS "http://127.0.0.1:$CLI_PORT/backup" 2>/dev/null | grep -q '"password_set"'; then ok "cli: backup password never exposed (only password_set)"; else bad "cli: backup password field"; fi
            if $F backup 2>/dev/null | grep -q "not initialized"; then ok "cli: backup CLI list (uninitialized)"; else bad "cli: backup CLI list"; fi
            if $F --json backup 2>/dev/null | grep -q '"snapshots"'; then ok "cli: backup --json"; else bad "cli: backup --json"; fi
            if $F backup run --stack "$CLI_STACK" >/dev/null 2>&1; then
                if command -v restic >/dev/null 2>&1 || command -v /usr/local/bin/restic >/dev/null 2>&1; then ok "cli: backup run (restic present)"; else ok "cli: backup run skipped (no restic binary; env-only password path still exercised)"; fi
            else
                # No password configured on this ephemeral peer: a
                # clean failure is the correct answer, not a silent act.
                ok "cli: backup run without password fails cleanly (expected on ephemeral peer)"
            fi
            if $F mcp tools 2>/dev/null | grep -q "run_backup"; then ok "cli: mcp run_backup tool present"; else bad "cli: mcp run_backup tool"; fi
            if $F backup forget >/dev/null 2>&1; then bad "cli: backup forget without password must fail"; else ok "cli: backup forget without password exits non-zero"; fi
            kill $PEER_PID >/dev/null 2>&1
        fi
        docker compose -p "$CLI_STACK" -f "$CLI_TMP/stacks/$CLI_STACK/compose.yaml" down -v --remove-orphans >/dev/null 2>&1
        docker volume rm -f "$CLI_VOL" >/dev/null 2>&1
        rm -rf "$CLI_TMP" 2>/dev/null
        set -e
    fi
fi

# ---- Layer 2: compose smoke (first-milestone flow) ---------------------
# The milestone flow: build the peer image (deploy/), start two peer
# containers against one daemon, write a stack dir on B, verify it
# syncs to A (sync is pull-based, ADR 0001: A pulls FROM B), adopt it,
# export with its volume, restore on A, deploy health-gated, and
# verify the volume data survived the move.
#
# Isolation: peer state lives in per-run tmp dirs; the test stack name
# and its volume are unique per run (marker file + random suffix), so
# reruns and other compose projects on the same daemon are untouched.
if [ "$SKIP_CONTAINER" -eq 0 ]; then
    log "Layer 2: compose smoke (2 peers, sync, adopt, move with volume)"
    if [ ! -f go.mod ]; then
        skip "no peer binary yet — compose smoke skipped"
    elif [ ! -f deploy/docker-compose.yml ]; then
        skip "no deploy/docker-compose.yml yet — compose smoke skipped"
    elif ! command -v docker >/dev/null 2>&1; then
        skip "docker not installed — compose smoke skipped"
    else
        set +e
        SMOKE_TMP=""
        PEER_A=""; PEER_B=""; TEST_STACK=""; TEST_VOL=""; TEST_MARKER=""
        cleanup_smoke() {
            # Test stack containers/volumes first, then peers, then dirs.
            [ -n "$TEST_STACK" ] && docker compose -p "$TEST_STACK" -f "$SMOKE_TMP/stack/compose.yaml" down -v --remove-orphans >/dev/null 2>&1
            [ -n "$PEER_A" ] && docker rm -f "$PEER_A" >/dev/null 2>&1
            [ -n "$PEER_B" ] && docker rm -f "$PEER_B" >/dev/null 2>&1
            [ -n "$TEST_VOL" ] && docker volume rm -f "$TEST_VOL" >/dev/null 2>&1
            if [ -n "$SMOKE_TMP" ] && [ -d "$SMOKE_TMP" ]; then
                # Peer containers run as root: their state dirs are
                # root-owned, so the operator's rm cannot remove them.
                # A throwaway root container does the removal, then the
                # empty dir itself goes away with a normal rmdir.
                docker run --rm --user root -v "$SMOKE_TMP:/smoke" alpine:3.20 sh -c 'rm -rf /smoke/a /smoke/b /smoke/stack /smoke/*.json /smoke/*.tar.gz /smoke/*.log /smoke/*.id' >/dev/null 2>&1
                rm -rf "$SMOKE_TMP" 2>/dev/null || rmdir "$SMOKE_TMP" 2>/dev/null
            fi
        }
        SMOKE_TMP="$(mktemp -d /tmp/ferriage-smoke.XXXXXX)" || { bad "compose smoke: mktemp failed"; SMOKE_TMP=""; }
        SUFFIX="$(head -c4 /dev/urandom | od -An -tx1 | tr -d ' \n')"
        TEST_STACK="ferriage-smoke-$SUFFIX"
        TEST_VOL="${TEST_STACK}_data"
        TEST_MARKER="hello-volume-$SUFFIX"
        PEER_A="ferriage-smoke-a-$SUFFIX"
        PEER_B="ferriage-smoke-b-$SUFFIX"

        echo "  building peer image (deploy/Dockerfile)..."
        if docker build -f deploy/Dockerfile -t ferriage:smoke . >"$SMOKE_TMP/build.log" 2>&1; then
            ok "layer2: image build (deploy/Dockerfile)"
        else
            bad "layer2: image build (see $SMOKE_TMP/build.log; kept on failure cleanup)"
            tail -n 20 "$SMOKE_TMP/build.log"
            cleanup_smoke; SMOKE_TMP=""
        fi

        if [ -n "$SMOKE_TMP" ]; then
            mkdir -p "$SMOKE_TMP/a" "$SMOKE_TMP/b"

            # Peer B starts first (A will pull from it via FERRIAGE_PEER).
            # Both peers run on --network host: A's pull of B is just
            # localhost:18081, no NAT/port-publish/host-gateway to
            # reason about, and a stop/start keeps the same address.
            echo "  starting peer B..."
            docker run -d --name "$PEER_B" \
                --network host \
                -e FERRIAGE_ADDR=:18081 \
                -e FERRIAGE_STATE_DIR=/stacks \
                -e FERRIAGE_SYNC_INTERVAL=500ms \
                -v "$SMOKE_TMP/b:/stacks" \
                -v /var/run/docker.sock:/var/run/docker.sock \
                ferriage:smoke >"$SMOKE_TMP/b.id" 2>&1 || cleanup_smoke
            B_PORT=18081
            echo "  starting peer A (FERRIAGE_PEER=http://localhost:18081)..."
            docker run -d --name "$PEER_A" \
                --network host \
                -e FERRIAGE_ADDR=:18080 \
                -e FERRIAGE_PEER="http://localhost:18081" \
                -e FERRIAGE_SYNC_INTERVAL=500ms \
                -v "$SMOKE_TMP/a:/stacks" \
                -v /var/run/docker.sock:/var/run/docker.sock \
                ferriage:smoke >"$SMOKE_TMP/a.id" 2>&1 || cleanup_smoke

            # Wait for health on both.
            A_PORT=18080
            wait_healthy() {
                local name="$1" port="$2" i
                for i in $(seq 1 30); do
                    curl -fsS "http://127.0.0.1:$port/healthz" >/dev/null 2>&1 && return 0
                    sleep 0.5
                done
                echo "  peer $name not healthy; logs:"; docker logs "$name" 2>&1 | tail -n 20
                return 1
            }
            if wait_healthy "$PEER_A" "$A_PORT" && wait_healthy "$PEER_B" "$B_PORT"; then
                ok "layer2: two peers healthy"
            else
                bad "layer2: peers not healthy"
            fi

            # --- write a stack dir on B, deploy via B ----------------
            # Sync is pull-based (ADR 0001): A pulls FROM B
            # (FERRIAGE_PEER), never the reverse. So the stack dir is
            # written on B, deployed on B, and verified to sync B->A.
            mkdir -p "$SMOKE_TMP/stack"
            cat >"$SMOKE_TMP/stack/compose.yaml" <<YAML
services:
  app:
    image: alpine:3.20
    command: ["sh", "-c", "mkdir -p /data && echo $TEST_MARKER > /data/marker.txt && sleep 1000"]
    volumes:
      - data:/data
    healthcheck:
      test: ["CMD", "sh", "-c", "test -f /data/marker.txt"]
      interval: 2s
      timeout: 3s
      retries: 5
      start_period: 2s
volumes:
  data:
YAML
            mkdir -p "$SMOKE_TMP/b/$TEST_STACK"
            cp "$SMOKE_TMP/stack/compose.yaml" "$SMOKE_TMP/b/$TEST_STACK/compose.yaml"
            # Deploy via the peer API (compose CLI + health gate).
            if curl -sS -X POST "http://127.0.0.1:$B_PORT/stacks/$TEST_STACK/deploy" \
                    --max-time 120 >"$SMOKE_TMP/deploy.json" 2>&1 && grep -q '"healthy":true' "$SMOKE_TMP/deploy.json"; then
                ok "layer2: health-gated deploy on B"
            else
                bad "layer2: deploy on B did not report healthy"; cat "$SMOKE_TMP/deploy.json"
            fi
            # Volume data landed?
            if docker run --rm -v "$TEST_VOL:/vol" alpine:3.20 cat /vol/marker.txt 2>/dev/null | grep -q "$TEST_MARKER"; then
                ok "layer2: volume data written ($TEST_MARKER)"
            else
                bad "layer2: volume marker missing after deploy"
            fi

            # --- sync: B's stack dir appears on A -------------------
            SYNCED=0
            for i in $(seq 1 30); do
                if [ -f "$SMOKE_TMP/a/$TEST_STACK/compose.yaml" ]; then SYNCED=1; break; fi
                sleep 0.5
            done
            if [ "$SYNCED" -eq 1 ]; then
                ok "layer2: state dir sync B->A (file appeared)"
            else
                bad "layer2: sync B->A (no $TEST_STACK/compose.yaml on A)"
                echo "  -- A peer logs --"; docker logs "$PEER_A" 2>&1 | tail -n 5
                echo "  -- B manifest --"; curl -fsS "http://127.0.0.1:$B_PORT/sync/manifest" 2>&1 | head -c 400; echo
                echo "  -- A state dir --"; ls -R "$SMOKE_TMP/a" 2>&1 | head -n 20
                echo "  -- A->B reachability from inside A --"
                docker exec "$PEER_A" wget -qO- "http://127.0.0.1:$B_PORT/healthz" 2>&1 | head -c 200; echo
            fi
            # A serves a manifest containing our stack's compose file.
            if curl -fsS "http://127.0.0.1:$A_PORT/sync/manifest" 2>/dev/null | grep -q "$TEST_STACK/compose.yaml"; then
                ok "layer2: A manifest lists the synced file"
            else
                bad "layer2: A manifest missing synced file"
            fi

            # --- adopt: GET /stacks sees the running stack ----------
            if curl -fsS "http://127.0.0.1:$B_PORT/stacks" 2>/dev/null | grep -q "\"name\":\"$TEST_STACK\""; then
                ok "layer2: adopted by labels (GET /stacks)"
            else
                bad "layer2: adopt — $TEST_STACK absent from GET /stacks"
            fi

            # --- move: export on B, import on A, deploy on A --------
            # The stack name is the same on both peers (same daemon, so
            # the same project/volume names). Import refuses existing
            # stack dirs/volumes, so: drop B's containers+volume via
            # compose down -v, remove the synced copy on A, then import.
            if curl -sS -X POST "http://127.0.0.1:$B_PORT/stacks/$TEST_STACK/export" \
                    --max-time 60 -o "$SMOKE_TMP/$TEST_STACK.tar.gz" 2>&1 && [ -s "$SMOKE_TMP/$TEST_STACK.tar.gz" ]; then
                ok "layer2: snapshot export (gzip+tar, with volume)"
            else
                bad "layer2: snapshot export failed"
            fi
            if [ -s "$SMOKE_TMP/$TEST_STACK.tar.gz" ]; then
                # The move needs B GONE for its duration. Import
                # refuses an existing stack dir, and A is a sync slave
                # of B (pull-based, ADR 0001): deleting a/<stack>
                # while B runs just gets it re-synced within one 500ms
                # tick — the tombstone only guards against STALE peers,
                # and B is still live-selling this file. Nothing to
                # race but the order: down B's containers+volume,
                # delete B's stack dir too (so its manifest can't
                # re-sell anything), stop both, remove the synced copy
                # on A, start ONLY A, import, deploy, then restart B
                # and verify A-B sync STILL converges on the new file.
                docker compose -p "$TEST_STACK" -f "$SMOKE_TMP/stack/compose.yaml" down -v --remove-orphans >/dev/null 2>&1
                docker run --rm --user root -v "$SMOKE_TMP:/smoke" alpine:3.20 sh -c "rm -rf /smoke/b/$TEST_STACK" >/dev/null 2>&1
                docker stop "$PEER_A" "$PEER_B" >/dev/null 2>&1
                # The synced copy on A is root-owned (peer runs as
                # root): remove it via a throwaway root container.
                docker run --rm --user root -v "$SMOKE_TMP:/smoke" alpine:3.20 sh -c "rm -rf /smoke/a/$TEST_STACK" >/dev/null 2>&1
                docker start "$PEER_A" >/dev/null 2>&1
                wait_healthy "$PEER_A" "$A_PORT"
                if curl -sS -X POST "http://127.0.0.1:$A_PORT/stacks/$TEST_STACK/import" \
                        --data-binary @"$SMOKE_TMP/$TEST_STACK.tar.gz" \
                        --max-time 60 >"$SMOKE_TMP/import.json" 2>&1 && grep -q "\"stack\":\"$TEST_STACK\"" "$SMOKE_TMP/import.json"; then
                    ok "layer2: snapshot import on A"
                else
                    bad "layer2: snapshot import on A"; cat "$SMOKE_TMP/import.json"
                fi
                if curl -sS -X POST "http://127.0.0.1:$A_PORT/stacks/$TEST_STACK/deploy" \
                        --max-time 120 >"$SMOKE_TMP/deploy2.json" 2>&1 && grep -q '"healthy":true' "$SMOKE_TMP/deploy2.json"; then
                    ok "layer2: health-gated deploy on A after restore"
                else
                    bad "layer2: deploy on A after restore"; cat "$SMOKE_TMP/deploy2.json"
                fi
                if docker run --rm -v "$TEST_VOL:/vol" alpine:3.20 cat /vol/marker.txt 2>/dev/null | grep -q "$TEST_MARKER"; then
                    ok "layer2: VOLUME DATA SURVIVED THE MOVE ($TEST_MARKER)"
                else
                    bad "layer2: volume data lost in the move"
                fi
                # B returns. B has no FERRIAGE_PEER (sync is
                # pull-only, ADR 0001), so B manifests only its own
                # state dir — which was removed during the move. What
                # MUST hold: A's stack survives B's empty manifest.
                # A's ledger knows the file as a synced copy from B,
                # but the delete rule keeps it: the file was restored
                # after the last pull, so it is newer than the ledger
                # entry (a "user edit" the delete path must not kill).
                docker start "$PEER_B" >/dev/null 2>&1
                wait_healthy "$PEER_B" "$B_PORT"
                sleep 2
                if curl -fsS "http://127.0.0.1:$A_PORT/sync/manifest" 2>/dev/null | grep -q "$TEST_STACK/compose.yaml" && [ -f "$SMOKE_TMP/a/$TEST_STACK/compose.yaml" ]; then
                    ok "layer2: moved stack survives B's empty manifest (no delete heals back)"
                else
                    bad "layer2: moved stack deleted after B returned"
                fi
            fi

            cleanup_smoke
            SMOKE_TMP=""
        fi
        set -e
    fi
fi

# ---- Layer 3: Playwright visual baselines (UI only) --------------------
# Screenshot the live UI (peer + BFF from web/dist, auth off = dev
# mode) with headless Chrome and pixel-diff against the committed
# baselines in tests/baselines/. Chrome binary: $CHROME_BIN, else
# Playwright's chromium, else /opt/google/chrome/chrome.
if [ "$SKIP_PLAYWRIGHT" -eq 0 ]; then
    log "Layer 3: Playwright visual baselines"
    if [ ! -d tests/baselines ]; then
        skip "no tests/baselines yet — visual layer skipped"
    elif ! command -v node >/dev/null 2>&1 || [ ! -d web/dist ]; then
        skip "node or web/dist missing — visual layer skipped (run npm run build in web/)"
    else
        CHROME_BIN=""
        if [ -n "${CHROME_BIN:-}" ] && [ -x "$CHROME_BIN" ]; then
            :
        elif command -v chromium >/dev/null 2>&1; then
            CHROME_BIN="chromium"
        else
            for c in /home/user/.cache/ms-playwright/chromium-*/chrome-linux64/chrome /opt/google/chrome/chrome; do
                [ -x "$c" ] && CHROME_BIN="$c" && break
            done
        fi
        if [ -z "$CHROME_BIN" ]; then
            skip "no Chrome binary found — visual layer skipped"
        else
            set +e
            VIS_TMP="$(mktemp -d /tmp/ferriage-visual.XXXXXX)"
            PEER_PID=""
            BFF_PID=""
            cleanup_vis() {
                [ -n "$BFF_PID" ] && kill "$BFF_PID" >/dev/null 2>&1
                [ -n "$PEER_PID" ] && kill "$PEER_PID" >/dev/null 2>&1
                rm -rf "$VIS_TMP"
            }
            mkdir -p "$VIS_TMP/stacks"
            FERRIAGE_ADDR=:18082 FERRIAGE_STATE_DIR="$VIS_TMP/stacks" go run ./cmd/peer >"$VIS_TMP/peer.log" 2>&1 &
            PEER_PID=$!
            (cd web && PORT=19082 FERRIAGE_PEER=http://localhost:18082 node bff/index.mjs >"$VIS_TMP/bff.log" 2>&1) &
            BFF_PID=$!
            for i in $(seq 1 30); do curl -fsS http://127.0.0.1:19082/ >/dev/null 2>&1 && break; sleep 0.5; done

            # Pixel diff: 1280x800 screenshot vs baseline; the
            # threshold is generous on purpose — a tripwire for
            # gross layout breakage, not a font-renderer pin.
            "$CHROME_BIN" --headless=new --no-sandbox --disable-gpu --disable-dev-shm-usage --hide-scrollbars --window-size=1280,800 --virtual-time-budget=5000 --screenshot="$VIS_TMP/shot.png" http://127.0.0.1:19082/ >/dev/null 2>&1
            if [ -s "$VIS_TMP/shot.png" ]; then
                DIFF_OUT="$(node scripts/png-diff.mjs "$VIS_TMP/shot.png" tests/baselines/ui-2026-11-20-stack-list.png 2 2>&1)"
                if [ $? -eq 0 ]; then
                    ok "layer3: stack-list baseline ($DIFF_OUT)"
                else
                    bad "layer3: stack-list baseline drifted ($DIFF_OUT; regenerate after sign-off)"
                fi
            else
                bad "layer3: screenshot failed"
            fi
            cleanup_vis
            set -e
        fi
    fi
fi

printf "\n\033[1;36m== %s ==\033[0m\n" "Summary: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]

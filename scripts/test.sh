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

# ---- Layer 2: compose smoke (first-milestone flow) ---------------------
if [ "$SKIP_CONTAINER" -eq 0 ]; then
    log "Layer 2: compose smoke (2 peers, sync, adopt, move with volume)"
    if [ ! -f go.mod ]; then
        skip "no peer binary yet — compose smoke skipped"
    elif [ ! -f deploy/docker-compose.yml ]; then
        skip "no deploy/docker-compose.yml yet — compose smoke skipped"
    else
        # TODO(first milestone): build peer image, run two containers,
        # verify state sync, adopt a test stack, move it WITH its
        # volume via export/import. Fail loudly until implemented.
        bad "compose smoke not implemented yet (first milestone)"
    fi
fi

# ---- Layer 3: Playwright visual baselines (UI only) --------------------
if [ "$SKIP_PLAYWRIGHT" -eq 0 ]; then
    log "Layer 3: Playwright visual baselines"
    if [ ! -d tests/baselines ]; then
        skip "no tests/baselines yet — visual layer skipped"
    else
        bad "playwright baselines not wired yet"
    fi
fi

printf "\n\033[1;36m== %s ==\033[0m\n" "Summary: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]

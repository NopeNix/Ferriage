#!/usr/bin/env bash
# Install the pre-commit hook that runs scripts/test.sh --fast before
# every commit. Idempotent: safe to run multiple times.
set -e

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
HOOK="$REPO_ROOT/.git/hooks/pre-commit"
mkdir -p "$(dirname "$HOOK")"

cat > "$HOOK" <<'HOOK_EOF'
#!/usr/bin/env bash
# Auto-installed by scripts/install-hooks.sh — do not edit by hand.
# Runs layer 1 (lint + unit) before every commit. Set SKIP_TESTS=1 to bypass.
if [ "${SKIP_TESTS:-0}" = "1" ]; then
    echo "pre-commit: SKIP_TESTS=1, skipping test routine."
    exit 0
fi
exec "$(git rev-parse --show-toplevel)/scripts/test.sh" --fast
HOOK_EOF
chmod +x "$HOOK"
echo "Installed: $HOOK"
echo "Use SKIP_TESTS=1 git commit ... to bypass (not recommended)."

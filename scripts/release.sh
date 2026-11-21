#!/bin/bash
# Ferriage release: build, tag, push, and GitHub-release the peer + UI.
#
# AGENTS.md §6: tag/release/image-push need EXPLICIT confirmation.
# This script never runs without a version argument from a human:
#
#   scripts/release.sh v0.1.0           # full ship (pushes + releases)
#   scripts/release.sh v0.1.0 --dry-run # build + tag locally, no push/release
#
# Prerequisites:
#   * clean working tree on the tag's HEAD
#   * scripts/test.sh --fast green (full routine recommended)
#   * docker login (Docker Hub) + gh auth for the release step
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="${1:-}"
DRY_RUN=0
[ "${2:-}" = "--dry-run" ] && DRY_RUN=1

if [ -z "$VERSION" ]; then
  echo "usage: scripts/release.sh vX.Y.Z [--dry-run]" >&2
  exit 1
fi
case "$VERSION" in
  v[0-9]*.[0-9]*.[0-9]*) ;;
  *) echo "version must look like v1.2.3 (got: $VERSION)" >&2; exit 1 ;;
esac

PEER_IMAGE="nopenix/ferriage"
UI_IMAGE="nopenix/ferriage-ui"
COMMIT="$(git rev-parse HEAD)"

echo "== Ferriage release $VERSION ($COMMIT) =="

if [ -n "$(git status --porcelain)" ]; then
  echo "error: working tree is not clean — commit first" >&2
  exit 1
fi
if git rev-parse "$VERSION" >/dev/null 2>&1; then
  echo "error: tag $VERSION already exists" >&2
  exit 1
fi

echo "== layer 1 (fast) =="
scripts/test.sh --fast

echo "== building peer image =="
docker build -f deploy/Dockerfile \
  --build-arg FERRIAGE_VERSION="$VERSION" \
  --build-arg FERRIAGE_COMMIT="$COMMIT" \
  -t "${PEER_IMAGE}:${VERSION}" -t "${PEER_IMAGE}:latest" .

echo "== building UI image =="
docker build -f deploy/Dockerfile.ui \
  --build-arg FERRIAGE_VERSION="$VERSION" \
  --build-arg FERRIAGE_COMMIT="$COMMIT" \
  -t "${UI_IMAGE}:${VERSION}" -t "${UI_IMAGE}:latest" .

echo "== verifying version stamping =="
docker run --rm --entrypoint ferriage "${PEER_IMAGE}:${VERSION}" --help >/dev/null
docker run --rm -d --name ferriage-release-check -p 127.0.0.1:0:8080 "${PEER_IMAGE}:${VERSION}" >/dev/null
PORT="$(docker port ferriage-release-check 8080 | head -1 | sed 's/.*://')"
sleep 2
HEALTH="$(curl -fsS "http://127.0.0.1:${PORT}/healthz")"
docker rm -f ferriage-release-check >/dev/null
echo "  peer /healthz: $HEALTH"
echo "$HEALTH" | grep -q "\"version\":\"${VERSION}\"" \
  || { echo "error: peer version not stamped ($HEALTH)" >&2; exit 1; }
echo "$HEALTH" | grep -q "\"commit\":\"${COMMIT:0:7}\|\"commit\":\"${COMMIT}\"" \
  || { echo "error: peer commit not stamped ($HEALTH)" >&2; exit 1; }

if [ "$DRY_RUN" = "1" ]; then
  echo "== dry run: built + stamped, nothing pushed/tagged/released =="
  echo "   docker images: ${PEER_IMAGE}:${VERSION} ${UI_IMAGE}:${VERSION}"
  exit 0
fi

# --- everything below is the irreversible ship: confirm explicitly ---
echo
echo "About to PUSH images, TAG $VERSION, and create the GitHub release."
echo "Type 'ship $VERSION' to confirm (anything else aborts):"
read -r CONFIRM
[ "$CONFIRM" = "ship $VERSION" ] || { echo "aborted"; exit 1; }

echo "== pushing images =="
docker push "${PEER_IMAGE}:${VERSION}"
docker push "${PEER_IMAGE}:latest"
docker push "${UI_IMAGE}:${VERSION}"
docker push "${UI_IMAGE}:latest"

echo "== tagging =="
git tag "$VERSION"
git push origin "$VERSION"

echo "== GitHub release =="
gh release create "$VERSION" \
  --title "Ferriage $VERSION" \
  --notes "Peer: \`docker pull ${PEER_IMAGE}:${VERSION}\` · UI: \`docker pull ${UI_IMAGE}:${VERSION}\` — see docs/deployment.md and deploy/docker-compose.yml."

echo "== shipped $VERSION =="

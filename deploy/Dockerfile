# syntax=docker/dockerfile:1
# Ferriage peer image (Phase 1.6). One image, one binary, every node.
#
# The peer needs, at runtime:
#   * the docker CLI + Compose plugin — deploy shells out to
#     `docker compose up -d` (session 009 decision; compose-go is a
#     Phase 3 evaluation)
#   * the Docker socket, bind-mounted — the daemon API is how the
#     peer lists containers, reads health, and copies volume data
#   * curl + ca-certificates — operator convenience (health probes,
#     debugging), not used by the binary itself
#
# Build:  docker build -f deploy/Dockerfile -t ferriage:dev .
# The repo root is the build context (Go module lives there).

FROM golang:1.25-alpine AS build

RUN apk add --no-cache git ca-certificates

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# CGO off: the peer is pure Go (docker SDK over a unix socket).
# Static binary, no libc to ship.
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags="-s -w \
        -X github.com/NopeNix/Ferriage/internal/version.Version=${FERRIAGE_VERSION:-dev} \
        -X github.com/NopeNix/Ferriage/internal/version.Commit=${FERRIAGE_COMMIT:-unknown}" \
      -o /ferriage-peer ./cmd/peer

FROM alpine:3.20

# docker-cli-compose is the Compose plugin binary (/usr/libexec/docker/cli-plugins).
RUN apk add --no-cache docker-cli docker-cli-compose ca-certificates curl

COPY --from=build /ferriage-peer /usr/local/bin/ferriage-peer

# /stacks is the state dir (Compose files = source of truth, CONCEPT § hard 6).
# The socket must be bind-mounted at runtime; the peer talks to the
# host daemon through it.
ENV FERRIAGE_ADDR=:8080 \
    FERRIAGE_STATE_DIR=/stacks
VOLUME ["/stacks"]
EXPOSE 8080
ENTRYPOINT ["ferriage-peer"]

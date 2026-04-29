# syntax=docker/dockerfile:1.7

# -----------------------------------------------------------------------------
# Stage 0: frontend
# -----------------------------------------------------------------------------
FROM node:22-alpine AS frontend

WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci --prefer-offline
COPY web/ ./
RUN npm run build

# -----------------------------------------------------------------------------
# Stage 1: Go build
# -----------------------------------------------------------------------------
# Alpine-based Go image keeps the builder small. modernc/sqlite is pure Go so
# we do NOT need cgo; this lets us emit a fully-static binary that runs on
# `scratch`.
FROM golang:1.26.2-alpine AS builder

WORKDIR /src

# Cache module downloads in their own layer.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy the rest of the source tree.
COPY . .

# Build a small, static, stripped binary.
#   CGO_DISABLED => pure-Go; modernc/sqlite does not need C.
#   -trimpath    => reproducible, no local paths in binary.
#   -ldflags     => strip symbol + DWARF tables (-s -w).
ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH
ENV CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build \
        -trimpath \
        -ldflags="-s -w -X main.version=${VERSION}" \
        -o /out/metareel \
        ./cmd/server

# -----------------------------------------------------------------------------
# Runtime stage
# -----------------------------------------------------------------------------
# `scratch` is the smallest possible base image (empty). Because the binary is
# fully static and we ship CA certs + tzdata from the builder, everything the
# app needs fits in ~15-20 MB total.
FROM scratch AS runtime

# Minimal filesystem scaffolding.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# App binary + migration files + compiled frontend.
COPY --from=builder /out/metareel        /app/metareel
COPY --from=builder /src/db/migrations   /app/db/migrations
COPY --from=frontend /web/dist           /app/web/dist

# A writable directory for the SQLite database file. When running under an
# unprivileged user, mount a named volume onto /app/data.
WORKDIR /app
VOLUME ["/app/data"]

# Drop privileges. User `65532:65532` is the distroless/nonroot convention –
# /etc/passwd isn't required for Go to run as a numeric UID.
USER 65532:65532

EXPOSE 8080

# Runtime defaults are provided by `internal/config` envDefault tags.
# Any environment variables provided at container runtime override those values.

ENTRYPOINT ["/app/metareel"]

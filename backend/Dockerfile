# ── Stage 1: Build ───────────────────────────────────────────────────────────
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Cache deps layer separately from source
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s" \
    -o securepatrol \
    ./cmd/securepatrol \
 && CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o signer ./cmd/signer

# ── Stage 2: Run ─────────────────────────────────────────────────────────────
# Alpine (not distroless) because the worker runs pg_dump for backups (D-015);
# pg_dump must match the server's major version (PostgreSQL 16).
FROM alpine:3.24

# Base image packages lag behind security fixes: upgrade them at build time
# (Trivy fails the build on fixable HIGH/CRITICAL findings).
RUN apk upgrade --no-cache && apk add --no-cache ca-certificates tzdata postgresql16-client \
    && addgroup -S app && adduser -S -G app -H -h /app app

WORKDIR /app
COPY --from=builder /app/securepatrol /app/securepatrol
# The chain-head signer (D-071) ships in this image but runs as its own
# container with its own uid (10010) and key volume: `/app/signer run`.
COPY --from=builder /app/signer /app/signer
RUN mkdir /keys && chown 10010:10010 /keys && chmod 0700 /keys
COPY --from=builder /app/migrations   /migrations

USER app

# 8080 = API (+ /metrics, internal only); 9091 = worker /health + /metrics.
EXPOSE 8080 9091

# `securepatrol` runs the API; `securepatrol worker` runs notifications + jobs.
ENTRYPOINT ["/app/securepatrol"]

# ── Stage 1: Build ───────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Cache deps layer separately from source
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s" \
    -o securepatrol \
    ./cmd/securepatrol

# ── Stage 2: Run ─────────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12

# Copy binary and migration files
COPY --from=builder /app/securepatrol  /securepatrol
COPY --from=builder /app/migrations    /migrations

USER nonroot:nonroot

EXPOSE 8080

ENTRYPOINT ["/securepatrol"]

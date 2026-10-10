# Build stage
FROM golang:1.22-alpine AS builder
WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Copy go mod files first for layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build with CGO_ENABLED=0 (P0-1: modernc.org/sqlite pure Go)
# -s -w strips debug info for smaller binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w -X main.version=${VERSION:-1.0.0}" \
    -o /app/gateway ./cmd/gateway

# Runtime stage: distroless static (no shell, no libc)
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app

# Copy binary and config
COPY --from=builder /app/gateway .
COPY config/default.yaml /app/config/default.yaml

# Create data directory for SQLite
USER root
RUN mkdir -p /data && chown nonroot:nonroot /data
USER nonroot

# Expose data-plane (8080) and admin-plane (8081)
EXPOSE 8080 8081

# Healthcheck in pure exec form (P2-10: no shell)
HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
  CMD ["/app/gateway", "healthcheck"]

ENTRYPOINT ["/app/gateway"]
CMD ["--config", "/app/config/default.yaml"]

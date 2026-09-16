# Multi-stage Dockerfile for Quiver
# Stage 1: Build the Go application
FROM golang:1.24.2-alpine AS builder

# Optional: CI passes the same nightly-<sha>/stable-X.Y.Z version it stamps on
# the released binaries. Left empty, the build falls back to `git describe`,
# same as `make build` locally.
ARG VERSION=""

# Set working directory
WORKDIR /app

# Install build dependencies. git is required at build time: it backs the
# `git describe` version fallback below.
RUN apk add --no-cache git ca-certificates tzdata

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application. An unstamped build (bare `go build`, no -ldflags)
# compiles in the literal version "dev", which the daemon treats as a local
# dev build and scopes its home directory into the current working
# directory instead of the real ~/.quiver — never something a deployed
# container should do. Mirrors the Makefile's local-build version logic.
RUN QUIVER_EPOCH=1775932380; \
    BUILD_ID=$(( ($(date +%s) - QUIVER_EPOCH) / 86400 )); \
    RESOLVED_VERSION=${VERSION:-$(git describe --tags --always --dirty --exclude='nightly*' 2>/dev/null || echo dev)}; \
    CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo \
      -ldflags "-X main.version=${RESOLVED_VERSION} -X main.buildID=${BUILD_ID}" \
      -o quiver ./cmd/quiver

# Stage 2: Create the final image
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1001 -S quiver && \
    adduser -u 1001 -S quiver -G quiver

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/quiver .

# Copy configuration files
COPY --from=builder /app/.gitignore ./.gitignore

RUN chown -R quiver:quiver /app

# Switch to non-root user
USER quiver

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the daemon, bound to the port EXPOSE/HEALTHCHECK above assume. The
# default host (unix://) would never satisfy the TCP healthcheck.
CMD ["./quiver", "daemon", "--host", "tcp://0.0.0.0:8080"]

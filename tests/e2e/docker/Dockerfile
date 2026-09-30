# End-to-end image: a real quiver binary driving real arrows on Linux.
#
#   make test-e2e-docker                      # linux/arm64 (default)
#   E2E_PLATFORM=linux/amd64 make test-e2e-docker
#
# Build context is the repository root; tests/e2e/docker/Dockerfile.dockerignore
# keeps it to the Go sources.

# The builder runs natively and cross-compiles, so an emulated target platform
# pays emulation only for running the suite, never for compiling it.
FROM --platform=$BUILDPLATFORM golang:1.26.2-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

# The commit the binary reports. Passed in by the Makefile: a git worktree's
# .git is a pointer file the build context cannot follow.
ARG QUIVER_COMMIT=unknown

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# A version of "dev" would scope the daemon's home to $CWD/.quiver, so the
# binary is stamped as a real build on the stable channel.
RUN export CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
 && go build \
      -ldflags "-X main.version=e2e -X main.commit=${QUIVER_COMMIT} -X main.channel=stable" \
      -o /out/quiver ./cmd/quiver \
 && go build -tags e2e -o /out/githost ./tests/e2e/docker/githost

FROM debian:stable-slim

RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates git curl jq openssl \
 && rm -rf /var/lib/apt/lists/*

# The moved-tag phase serves its fixture repository from https://localhost,
# so the certificate that server presents is trusted system-wide: go-git
# (inside the daemon) and git (inside run.sh) both read the system store.
RUN mkdir -p /etc/githost \
 && openssl req -x509 -nodes -newkey rsa:2048 -days 3650 \
      -subj "/CN=localhost" \
      -addext "subjectAltName=DNS:localhost,IP:127.0.0.1" \
      -keyout /etc/githost/key.pem -out /etc/githost/cert.pem 2>/dev/null \
 && cp /etc/githost/cert.pem /usr/local/share/ca-certificates/quiver-e2e.crt \
 && update-ca-certificates >/dev/null

RUN useradd --create-home --home-dir /home/quiver --shell /bin/bash quiver \
 && mkdir -p /srv/git \
 && chown quiver:quiver /srv/git /etc/githost/key.pem

COPY --from=builder /out/quiver /out/githost /usr/local/bin/
COPY tests/e2e/docker/run.sh /usr/local/bin/run-e2e
COPY tests/e2e/docker/fixture /opt/e2e/fixture

USER quiver
WORKDIR /home/quiver

ENV HOME=/home/quiver \
    QUIVER_HOME=/home/quiver/.quiver

CMD ["/usr/local/bin/run-e2e"]

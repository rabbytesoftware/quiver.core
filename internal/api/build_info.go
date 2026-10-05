package api

import "github.com/rabbytesoftware/quiver.core/internal/api/endpoints/versions"

// BuildInfo carries build-time version data injected via ldflags, and the
// optional features the daemon serves.
type BuildInfo = versions.Build

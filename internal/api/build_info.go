package api

// BuildInfo carries build-time version data injected via ldflags.
type BuildInfo struct {
	Version string
	BuildID string
}

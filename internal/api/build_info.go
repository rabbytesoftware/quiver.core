package api

// BuildInfo carries build-time version data injected via ldflags.
type BuildInfo struct {
	Version string
	BuildID string
	Commit  string
	BuiltAt string
	Channel string

	// Features lists the optional capabilities this daemon serves, advertised
	// on GET /versions so a client can tell what an older daemon lacks.
	Features []string
}

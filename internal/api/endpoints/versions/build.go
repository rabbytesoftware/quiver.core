package versions

// Build is the build-time identity of the running daemon.
type Build struct {
	Version string
	BuildID string
	Commit  string
	BuiltAt string
	Channel string
}

package command

// Options configures an Executor.
type Options struct {
	// ServerURI returns the address commands dial to reach this daemon, for
	// example "unix:///home/u/.quiver/quiver.sock" or "tcp://127.0.0.1:40257".
	// It is read on every call, so the address may be set after construction.
	// An empty result makes Prepare fail with ErrUnavailable.
	ServerURI func() string

	// Version is the daemon's own version, shown by the version command.
	Version string
}

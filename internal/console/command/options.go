package command

// Options configures an Executor.
type Options struct {
	// ServerURI is the address commands dial to reach this daemon, for example
	// "unix:///home/u/.quiver/quiver.sock" or "tcp://127.0.0.1:40257". Empty
	// makes Prepare fail with ErrUnavailable.
	ServerURI string

	// Version is the daemon's own version, shown by the version command.
	Version string
}

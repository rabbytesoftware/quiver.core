package v0

type options struct {
	version string
}

// Option configures the v0 container.
type Option func(*options)

// WithVersion sets the daemon's own version, which the console's version
// command reports as the client version.
func WithVersion(
	version string,
) Option {
	return func(o *options) { o.version = version }
}

package console

import "time"

// Option adjusts a Handlers at construction.
type Option func(*Handlers)

// WithExecTimeout sets how long one command may run.
func WithExecTimeout(
	d time.Duration,
) Option {
	return func(h *Handlers) { h.execTimeout = d }
}

// WithOutputLimit sets how many bytes of output one command call streams.
func WithOutputLimit(
	bytes int,
) Option {
	return func(h *Handlers) { h.outputLimit = bytes }
}

// WithLimiter replaces the command concurrency limiter.
func WithLimiter(
	l ExecLimiter,
) Option {
	return func(h *Handlers) { h.limiter = l }
}

// WithStuckLimit sets how long a log consumer's queue may stay full before
// the stream is closed.
func WithStuckLimit(
	d time.Duration,
) Option {
	return func(h *Handlers) { h.stuckLimit = d }
}

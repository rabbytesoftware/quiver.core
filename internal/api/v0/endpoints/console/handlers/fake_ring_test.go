package console

import (
	"log/slog"

	"github.com/rabbytesoftware/quiver.core/internal/core/logring"
)

type fakeRing struct {
	stream *fakeStream
}

func (r *fakeRing) Wrap(
	next slog.Handler,
) slog.Handler {
	return next
}

func (r *fakeRing) Stream(
	_ uint64,
	_ slog.Level,
) logring.Stream {
	return r.stream
}

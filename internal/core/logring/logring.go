// Package logring captures the daemon's own slog records as structured data
// in a bounded ring buffer and fans them out to live subscribers. It is the
// source of the console log stream. internal.New builds one ring per daemon and
// hands it to the logger (which tees into it) and to the console handlers
// (which read from it); nothing here is process-global.
//
// Records are captured from slog.Record values rather than parsed from the
// daemon's output, so the stream does not depend on whether the file logger
// is enabled or whether the configured handler writes JSON or text. Sensitive
// attribute values are redacted before a record is stored, which covers replay
// and live delivery alike.
package logring

// DefaultCapacity is the number of records the daemon's ring retains.
const DefaultCapacity = 5000

// New returns an empty ring that retains the last capacity records. A
// capacity below one is raised to one.
func New(
	capacity int,
) Ring {
	if capacity < 1 {
		capacity = 1
	}
	return &ringBuffer{
		entries: make([]Record, capacity),
		subs:    make(map[*queueSubscription]struct{}),
	}
}

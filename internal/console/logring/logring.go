// Package logring captures the daemon's own slog records as structured data
// in a bounded, process-wide ring buffer and fans them out to live
// subscribers. It is the source of the console log stream.
//
// Records are captured from slog.Record values rather than parsed from the
// daemon's output, so the stream does not depend on whether the file logger
// is enabled or whether the configured handler writes JSON or text. Sensitive
// attribute values are redacted before a record is stored, which covers replay
// and live delivery alike.
package logring

// DefaultCapacity is the number of records the process-wide ring retains.
const DefaultCapacity = 5000

var defaultRing = New(DefaultCapacity)

// Default returns the process-wide ring the daemon's logger tees into and the
// console log stream reads from.
func Default() Ring {
	return defaultRing
}

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

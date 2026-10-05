// Package logring keeps the daemon's most recent log records in memory so the
// console can replay them and follow new ones.
package logring

import (
	"log/slog"
	"regexp"
)

const (
	capacity     = 5000
	liveBuffer   = 256
	replayLast   = 500
	maxValue     = 2048
	maxAttrs     = 32
	redactedText = "[redacted]"
	sensitiveKey = `(?i)token|secret|passw(or)?d|authorization|api[-_]?key|private[-_]?key|bearer|cookie|credential|pairing`
)

// New returns an empty Ring that retains the newest 5000 records.
//
// Each Ring owns its own storage and its own redaction pattern, so two rings
// never share state and nothing is kept at package level. The returned Ring is
// safe for concurrent use by any number of loggers and readers.
func New() Ring {
	return &ringBuffer{
		records:     make([]Record, capacity),
		subscribers: make(map[*subscriber]struct{}),
		redact:      regexp.MustCompile(sensitiveKey),
	}
}

var _ slog.Handler = (*handler)(nil)

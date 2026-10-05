package logring

import (
	"log/slog"
	"time"
)

// Record is one captured log record in the shape the console streams it, so it
// can be encoded to JSON as is.
//
// Seq is the position of the record in this process's log, starting at 1 and
// never reused, so a client can resume with the last Seq it saw. Time is when
// the record was logged. Level is the lower-case slog level name such as
// "warn". Component is the value of the record's "component" attribute, or the
// empty string when it has none. Msg is the log message. Fields holds the
// remaining attributes with group names joined to keys by dots: values of
// sensitive keys are replaced by "[redacted]", strings are cut at 2 KiB and a
// record keeps at most 32 fields. Type is always "log" so a client can tell a
// record from a control frame.
type Record struct {
	Type      string         `json:"type"`
	Seq       uint64         `json:"seq"`
	Time      time.Time      `json:"time"`
	Level     string         `json:"level"`
	Component string         `json:"component"`
	Msg       string         `json:"msg"`
	Fields    map[string]any `json:"fields"`

	level slog.Level
}

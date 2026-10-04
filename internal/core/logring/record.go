package logring

import "time"

// Record is one captured log record in the shape the console stream sends.
//
// Fields holds every attribute except component, flattened with dotted keys
// for nested groups. Values keep their JSON type: strings, numbers and
// booleans. Durations and errors are rendered to strings.
type Record struct {
	Seq             uint64         `json:"seq"`
	Time            time.Time      `json:"time"`
	Level           string         `json:"level"`
	Component       string         `json:"component"`
	Msg             string         `json:"msg"`
	Fields          map[string]any `json:"fields"`
	FieldsTruncated bool           `json:"fields_truncated"`
}

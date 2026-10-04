package logring

import "time"

// Record is one captured log record in the shape the console stream sends.
//
// Fields holds every attribute except component, flattened with dotted keys
// for nested groups. Values keep their JSON type: strings, numbers and
// booleans. Durations and errors are rendered to strings.
type Record struct {
	Seq             uint64         `json:"seq" yaml:"seq"`
	Time            time.Time      `json:"time" yaml:"time"`
	Level           string         `json:"level" yaml:"level"`
	Component       string         `json:"component" yaml:"component"`
	Msg             string         `json:"msg" yaml:"msg"`
	Fields          map[string]any `json:"fields" yaml:"fields"`
	FieldsTruncated bool           `json:"fields_truncated" yaml:"fields_truncated"`
}

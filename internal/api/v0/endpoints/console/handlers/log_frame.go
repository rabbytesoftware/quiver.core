package console

import "github.com/rabbytesoftware/quiver.core/internal/core/logring"

type logFrame struct {
	Type string `json:"type" yaml:"type"`
	logring.Record
}

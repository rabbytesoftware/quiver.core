package console

import "github.com/rabbytesoftware/quiver.core/internal/console/logring"

type logFrame struct {
	Type string `json:"type"`
	logring.Record
}

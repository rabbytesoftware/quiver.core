package store

import (
	"fmt"

	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/store/sqlite"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/ports"
)

// NewPortSQLite returns a SQLite-backed PortStore at path.
func NewPortSQLite(path string) (PortStore, error) {
	inner, err := sqlite.New[ports.PortAllocation](path, "port_allocations", "port")
	if err != nil {
		return nil, fmt.Errorf("port sqlite: %w", err)
	}
	return &portStore{inner: inner}, nil
}

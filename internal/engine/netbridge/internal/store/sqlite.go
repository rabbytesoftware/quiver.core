package store

import (
	"fmt"

	adapterSQLite "github.com/rabbytesoftware/quiver/internal/adapter/store/sqlite"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/ports"
)

func NewPortSQLite(path string) (PortStore, error) {
	inner, err := adapterSQLite.New[ports.PortAllocation, int](path)
	if err != nil {
		return nil, fmt.Errorf("port sqlite: %w", err)
	}
	return &portStore{inner: inner}, nil
}

package store

import (
	"embed"
	"fmt"

	adapterSQLite "github.com/rabbytesoftware/quiver/internal/adapter/store/sqlite"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/ports"
)

//go:embed migrations/*.sql
var migrations embed.FS

func NewPortSQLite(path string) (PortStore, error) {
	db, err := adapterSQLite.Open(path, migrations, "migrations")
	if err != nil {
		return nil, fmt.Errorf("port sqlite: %w", err)
	}
	return &portStore{inner: adapterSQLite.New[ports.PortAllocation, int](db, "port_allocations", "port")}, nil
}

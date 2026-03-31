package store

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"

	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/ports"
)

const schema = `
CREATE TABLE IF NOT EXISTS port_allocations (
	port      INTEGER PRIMARY KEY,
	protocol  TEXT    NOT NULL,
	owner_key TEXT    NOT NULL,
	forwarded INTEGER NOT NULL DEFAULT 0
);`

type sqliteStore struct {
	db *sql.DB
}

// NewSQLite opens (or creates) a SQLite database at path and returns a Store
// backed by it. The schema is applied automatically on first open.
func NewSQLite(
	path string,
) (Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("store: open sqlite: %w", err)
	}

	if _, err = db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: apply schema: %w", err)
	}

	return &sqliteStore{db: db}, nil
}

func (s *sqliteStore) Save(
	allocation ports.PortAllocation,
) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO port_allocations (port, protocol, owner_key, forwarded)
		 VALUES (?, ?, ?, ?)`,
		allocation.Port,
		string(allocation.Protocol),
		allocation.OwnerKey,
		boolToInt(allocation.Forwarded),
	)
	return err
}

func (s *sqliteStore) Delete(
	port int,
) error {
	_, err := s.db.Exec(
		`DELETE FROM port_allocations WHERE port = ?`,
		port,
	)
	return err
}

func (s *sqliteStore) FindByPort(
	port int,
) (*ports.PortAllocation, error) {
	row := s.db.QueryRow(
		`SELECT port, protocol, owner_key, forwarded
		 FROM port_allocations WHERE port = ?`,
		port,
	)

	var alloc ports.PortAllocation
	var forwarded int
	err := row.Scan(&alloc.Port, &alloc.Protocol, &alloc.OwnerKey, &forwarded)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	alloc.Forwarded = forwarded != 0
	return &alloc, nil
}

func (s *sqliteStore) FindByOwner(
	ownerKey string,
) ([]ports.PortAllocation, error) {
	rows, err := s.db.Query(
		`SELECT port, protocol, owner_key, forwarded
		 FROM port_allocations WHERE owner_key = ?`,
		ownerKey,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]ports.PortAllocation, 0, 4)
	for rows.Next() {
		var alloc ports.PortAllocation
		var forwarded int
		if err := rows.Scan(&alloc.Port, &alloc.Protocol, &alloc.OwnerKey, &forwarded); err != nil {
			return nil, err
		}
		alloc.Forwarded = forwarded != 0
		result = append(result, alloc)
	}
	return result, rows.Err()
}

func boolToInt(
	b bool,
) int {
	if b {
		return 1
	}
	return 0
}

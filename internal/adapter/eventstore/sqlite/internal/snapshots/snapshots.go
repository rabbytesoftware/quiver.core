package snapshots

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite/internal/conn"
	"github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite/internal/entries"
)

// New opens the snapshots database at path, in its own SQLite file rather
// than sharing the event store's.
//
// The event store is opened single-connection and write-pinned, and
// reader.Load issues Get first on every read. A snapshots table living in that
// same database would serialize the O(1) Get behind any in-flight Append. asynx
// never wraps the event Append and the snapshot Put in a shared transaction,
// so co-locating the tables would not buy atomicity in exchange for that cost.
func New(
	path string,
) (*Store, error) {
	db, err := conn.Open(path)
	if err != nil {
		return nil, fmt.Errorf("snapshotstore: open: %w", err)
	}

	err = prepare(db)
	if err != nil {
		conn.Close(db)
		return nil, err
	}

	return &Store{db: db}, nil
}

func prepare(
	db *gorm.DB,
) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("snapshotstore: db: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)

	err = db.Exec("PRAGMA busy_timeout=5000").Error
	if err != nil {
		return fmt.Errorf("snapshotstore: busy_timeout: %w", err)
	}

	err = db.AutoMigrate(&entries.Snapshot{})
	if err != nil {
		return fmt.Errorf("snapshotstore: migrate: %w", err)
	}

	return nil
}

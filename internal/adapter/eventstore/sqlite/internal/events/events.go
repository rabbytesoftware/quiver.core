package events

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite/internal/conn"
	"github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite/internal/entries"
)

// New opens the events database at path, creating or upgrading its tables. A
// database written by an earlier release only gains the receipts and
// deliveries tables.
func New(
	path string,
) (*Store, error) {
	db, err := conn.Open(path)
	if err != nil {
		return nil, fmt.Errorf("eventstore: open: %w", err)
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
		return fmt.Errorf("eventstore: db: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)

	err = db.Exec("PRAGMA busy_timeout=5000").Error
	if err != nil {
		return fmt.Errorf("eventstore: busy_timeout: %w", err)
	}

	err = db.AutoMigrate(&entries.Event{}, &entries.Receipt{}, &entries.Delivery{})
	if err != nil {
		return fmt.Errorf("eventstore: migrate: %w", err)
	}

	return nil
}

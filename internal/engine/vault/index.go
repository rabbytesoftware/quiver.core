package vault

import (
	"fmt"

	"gorm.io/gorm"

	adapterSQLite "github.com/rabbytesoftware/quiver.core/internal/adapter/store/sqlite"
)

type index struct {
	db *gorm.DB
}

// openIndex opens the vault's read model, creating its schema if absent.
// The FTS5 table is created with raw DDL because GORM cannot AutoMigrate
// virtual tables.
func openIndex(dbPath string) (*index, error) {
	db, err := adapterSQLite.OpenDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("vault index: open: %w", err)
	}

	if err := db.AutoMigrate(
		&arrowIndexRow{},
		&arrowTagRow{},
		&arrowOSRow{},
	); err != nil {
		return nil, fmt.Errorf("vault index: migrate: %w", err)
	}

	if err := db.Exec(
		`CREATE VIRTUAL TABLE IF NOT EXISTS vault_arrows_fts USING fts5(
			namespace UNINDEXED,
			ref UNINDEXED,
			name,
			description,
			tags,
			tokenize = 'trigram'
		)`,
	).Error; err != nil {
		return nil, fmt.Errorf("vault index: create fts: %w", err)
	}

	return &index{db: db}, nil
}

func (i *index) close() error {
	sqlDB, err := i.db.DB()
	if err != nil {
		return fmt.Errorf("vault index: close: %w", err)
	}
	return sqlDB.Close()
}

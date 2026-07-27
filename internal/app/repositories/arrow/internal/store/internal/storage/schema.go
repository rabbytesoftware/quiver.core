package storage

import (
	"fmt"

	gormdb "gorm.io/gorm"
)

// newSchema creates the catalog read model. The FTS5 table is created with raw
// DDL because GORM cannot AutoMigrate virtual tables.
func newSchema(
	db *gormdb.DB,
) error {
	if err := db.AutoMigrate(
		&arrowRow{},
		&arrowVersionRow{},
		&arrowTagRow{},
		&arrowOSRow{},
	); err != nil {
		return fmt.Errorf("storage: migrate: %w", err)
	}

	if err := db.Exec(
		`CREATE VIRTUAL TABLE IF NOT EXISTS catalog_arrows_fts USING fts5(
			namespace UNINDEXED,
			name,
			description,
			tags,
			tokenize = 'trigram'
		)`,
	).Error; err != nil {
		return fmt.Errorf("storage: create fts: %w", err)
	}

	return nil
}

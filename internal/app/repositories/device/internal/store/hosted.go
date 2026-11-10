package store

import (
	"context"
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"
	gormdb "gorm.io/gorm"
	"gorm.io/gorm/clause"

	eventstore "github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver.core/internal/adapter/store/sqlite"
	"github.com/rabbytesoftware/quiver.core/internal/domain/auth"
)

// NewHosted constructs a Store whose table lives in the device aggregate's
// events database host, so UpsertTx commits with the event it projects. Rows
// a previous release kept in its own database, legacy, are copied in; legacy
// may be nil. A repeat is harmless: a device already present is never
// overwritten by its older legacy row.
func NewHosted(
	host *gormdb.DB,
	legacy *gormdb.DB,
) (TxStore, error) {
	if host == nil {
		return nil, fmt.Errorf("device store: host db is required")
	}
	err := host.AutoMigrate(&deviceRow{})
	if err != nil {
		return nil, fmt.Errorf("device store: migrate: %w", err)
	}
	_, err = sqlite.ImportLegacyDB[deviceRow](host, legacy)
	if err != nil {
		return nil, fmt.Errorf("device store: import legacy rows: %w", err)
	}
	return &gormStore{db: host}, nil
}

func (s *gormStore) UpsertTx(
	ctx context.Context,
	tx asynxModels.Tx,
	d auth.Device,
) error {
	session, err := eventstore.TxSession(ctx, s.db, tx)
	if err != nil {
		return fmt.Errorf("device store: upsert: %w", err)
	}
	row := toRow(d)
	err = session.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		UpdateAll: true,
	}).Create(&row).Error
	if err != nil {
		return fmt.Errorf("device store: upsert: %w", err)
	}
	return nil
}

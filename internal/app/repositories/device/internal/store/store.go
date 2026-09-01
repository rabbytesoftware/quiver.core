package store

import (
	"context"
	"fmt"
	"time"

	gormdb "gorm.io/gorm"
	"gorm.io/gorm/clause"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/domain/auth"
)

// deviceRow is the read model for one paired device. Authenticate looks a
// device up by token hash, not by ID, which Asynx cannot index — this table
// exists purely to serve that lookup.
type deviceRow struct {
	ID         string `gorm:"primaryKey;column:id"`
	Label      string `gorm:"column:label"`
	TokenHash  string `gorm:"column:token_hash;uniqueIndex"`
	State      string `gorm:"column:state"`
	PairedAt   int64  `gorm:"column:paired_at"`
	LastSeenAt int64  `gorm:"column:last_seen_at"`
}

func (deviceRow) TableName() string { return "auth_devices" }

// Store is the read model kept in step with the device aggregate.
type Store interface {
	// Upsert writes the full current state of one device, creating it on
	// first pairing and overwriting on every later change.
	Upsert(
		ctx context.Context,
		d auth.Device,
	) error
	Get(
		ctx context.Context,
		deviceID string,
	) (auth.Device, error)
	// GetByTokenHash is the hot path: every authenticated tcp:// request looks
	// a device up by its bearer token's hash.
	GetByTokenHash(
		ctx context.Context,
		tokenHash string,
	) (auth.Device, error)
	List(
		ctx context.Context,
	) ([]auth.Device, error)
}

type gormStore struct {
	db *gormdb.DB
}

// New constructs a Store, migrating the auth_devices table if needed.
func New(
	db *gormdb.DB,
) (Store, error) {
	if db == nil {
		return nil, fmt.Errorf("device store: db is required")
	}

	if err := db.AutoMigrate(&deviceRow{}); err != nil {
		return nil, fmt.Errorf("device store: migrate: %w", err)
	}

	return &gormStore{db: db}, nil
}

func (s *gormStore) Upsert(
	ctx context.Context,
	d auth.Device,
) error {
	row := toRow(d)

	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		UpdateAll: true,
	}).Create(&row).Error; err != nil {
		return fmt.Errorf("device store: upsert: %w", err)
	}

	return nil
}

func (s *gormStore) Get(
	ctx context.Context,
	deviceID string,
) (auth.Device, error) {
	var rows []deviceRow
	if err := s.db.WithContext(ctx).Where("id = ?", deviceID).Find(&rows).Error; err != nil {
		return auth.Device{}, fmt.Errorf("device store: get: %w", err)
	}
	if len(rows) == 0 {
		return auth.Device{}, apperrors.ErrNotFound
	}

	return fromRow(rows[0]), nil
}

func (s *gormStore) GetByTokenHash(
	ctx context.Context,
	tokenHash string,
) (auth.Device, error) {
	var rows []deviceRow
	if err := s.db.WithContext(ctx).Where("token_hash = ?", tokenHash).Find(&rows).Error; err != nil {
		return auth.Device{}, fmt.Errorf("device store: get by token hash: %w", err)
	}
	if len(rows) == 0 {
		return auth.Device{}, apperrors.ErrNotFound
	}

	return fromRow(rows[0]), nil
}

func (s *gormStore) List(
	ctx context.Context,
) ([]auth.Device, error) {
	var rows []deviceRow
	if err := s.db.WithContext(ctx).Order("paired_at DESC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("device store: list: %w", err)
	}

	devices := make([]auth.Device, 0, len(rows))
	for _, row := range rows {
		devices = append(devices, fromRow(row))
	}

	return devices, nil
}

func toRow(d auth.Device) deviceRow {
	return deviceRow{
		ID:         d.ID,
		Label:      d.Label,
		TokenHash:  d.TokenHash,
		State:      string(d.State),
		PairedAt:   d.PairedAt.UnixNano(),
		LastSeenAt: d.LastSeenAt.UnixNano(),
	}
}

func fromRow(row deviceRow) auth.Device {
	return auth.Device{
		ID:         row.ID,
		Label:      row.Label,
		TokenHash:  row.TokenHash,
		State:      auth.DeviceState(row.State),
		PairedAt:   time.Unix(0, row.PairedAt).UTC(),
		LastSeenAt: time.Unix(0, row.LastSeenAt).UTC(),
	}
}

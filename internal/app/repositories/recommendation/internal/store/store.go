package store

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/recommendation/internal/store/internal/storage"
)

// Store is the snapshot read model: a derived copy of what the last refresh
// found, replaced a shelf at a time.
type Store interface {
	// Snapshot returns a shelf's entries in rank order. An unknown shelf is an
	// empty snapshot, not an error.
	Snapshot(
		ctx context.Context,
		shelfID string,
	) (Snapshot, error)

	// Replace swaps a shelf's whole snapshot in one transaction, so a reader
	// sees the old list or the new one and never a mixture, and a failure leaves
	// the old one in place.
	Replace(
		ctx context.Context,
		shelfID string,
		at time.Time,
		entries []Entry,
	) error

	// Retain deletes the snapshots of every shelf not named, which are the ones
	// the configuration no longer has.
	Retain(
		ctx context.Context,
		shelfIDs []string,
	) error
}

type gormStore struct {
	db *gorm.DB
}

// New builds the snapshot store over db, creating its tables.
func New(
	db *gorm.DB,
) (Store, error) {
	if db == nil {
		return nil, fmt.Errorf("recommendation store: db must not be nil")
	}
	if err := db.AutoMigrate(&storage.EntryRow{}, &storage.ShelfRow{}); err != nil {
		return nil, fmt.Errorf("recommendation store: migrate: %w", err)
	}
	return &gormStore{db: db}, nil
}

func (s *gormStore) Snapshot(
	ctx context.Context,
	shelfID string,
) (Snapshot, error) {
	var shelf storage.ShelfRow
	found := s.db.WithContext(ctx).Limit(1).Find(&shelf, "shelf_id = ?", shelfID)
	if found.Error != nil {
		return Snapshot{}, fmt.Errorf("recommendation store: read shelf %s: %w", shelfID, found.Error)
	}

	var rows []storage.EntryRow
	if err := s.db.WithContext(ctx).Order("position").Find(&rows, "shelf_id = ?", shelfID).Error; err != nil {
		return Snapshot{}, fmt.Errorf("recommendation store: read entries %s: %w", shelfID, err)
	}

	entries := make([]Entry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, Entry{Namespace: row.Namespace, Stars: row.Stars, Source: row.Source})
	}
	return Snapshot{RefreshedAt: shelf.RefreshedAt, Entries: entries}, nil
}

func (s *gormStore) Replace(
	ctx context.Context,
	shelfID string,
	at time.Time,
	entries []Entry,
) error {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return replaceShelf(tx, shelfID, at.UTC(), entries)
	})
	if err != nil {
		return fmt.Errorf("recommendation store: replace shelf %s: %w", shelfID, err)
	}
	return nil
}

func replaceShelf(
	tx *gorm.DB,
	shelfID string,
	at time.Time,
	entries []Entry,
) error {
	if err := tx.Delete(&storage.EntryRow{}, "shelf_id = ?", shelfID).Error; err != nil {
		return err
	}
	if len(entries) > 0 {
		if err := tx.Create(entryRows(shelfID, entries)).Error; err != nil {
			return err
		}
	}
	return tx.Clauses(clause.OnConflict{UpdateAll: true}).
		Create(&storage.ShelfRow{ShelfID: shelfID, RefreshedAt: at}).Error
}

func entryRows(
	shelfID string,
	entries []Entry,
) []storage.EntryRow {
	rows := make([]storage.EntryRow, 0, len(entries))
	for position, entry := range entries {
		rows = append(rows, storage.EntryRow{
			ShelfID:   shelfID,
			Position:  position,
			Namespace: entry.Namespace,
			Stars:     entry.Stars,
			Source:    entry.Source,
		})
	}
	return rows
}

func (s *gormStore) Retain(
	ctx context.Context,
	shelfIDs []string,
) error {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return dropOtherShelves(tx, shelfIDs)
	})
	if err != nil {
		return fmt.Errorf("recommendation store: retain shelves: %w", err)
	}
	return nil
}

func dropOtherShelves(
	tx *gorm.DB,
	shelfIDs []string,
) error {
	if len(shelfIDs) == 0 {
		if err := tx.Where("1 = 1").Delete(&storage.EntryRow{}).Error; err != nil {
			return err
		}
		return tx.Where("1 = 1").Delete(&storage.ShelfRow{}).Error
	}
	if err := tx.Where("shelf_id NOT IN ?", shelfIDs).Delete(&storage.EntryRow{}).Error; err != nil {
		return err
	}
	return tx.Where("shelf_id NOT IN ?", shelfIDs).Delete(&storage.ShelfRow{}).Error
}

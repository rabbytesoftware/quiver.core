package sqlite

import (
	"context"
	"io"

	asynxModels "github.com/char2cs/asynx/models"
	"gorm.io/gorm"
)

// Store is a SQLite-backed event store that extends the asynx store interface with lifecycle management.
type Store interface {
	asynxModels.Store
	io.Closer

	// ListAggregateIDs returns the distinct aggregate IDs that currently have
	// events in the store, with the internal "events:" key prefix stripped.
	ListAggregateIDs(ctx context.Context) ([]string, error)
}

// SnapshotStore is a SQLite-backed asynx snapshot store with lifecycle management.
type SnapshotStore interface {
	asynxModels.SnapshotStore
	io.Closer
}

// closeDB releases a handle whose constructor failed after gorm.Open succeeded,
// best effort. Without it the pooled connection stays open with nothing left to
// close it, since no store was returned to the caller.
func closeDB(db *gorm.DB) {
	sqlDB, err := db.DB()
	if err != nil {
		return
	}
	_ = sqlDB.Close()
}

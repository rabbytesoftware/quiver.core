package sqlite

import (
	"io"

	asynxModels "github.com/char2cs/asynx/models"
)

// Store is a SQLite-backed event store that extends the asynx store interface with lifecycle management.
type Store interface {
	asynxModels.Store
	io.Closer
}

// SnapshotStore is a SQLite-backed asynx snapshot store with lifecycle management.
type SnapshotStore interface {
	asynxModels.SnapshotStore
	io.Closer
}

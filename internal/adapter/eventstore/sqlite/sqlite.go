package sqlite

import (
	"github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite/internal/events"
	"github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite/internal/snapshots"
)

// NewEventStore returns a GORM-backed asynx event store. It also satisfies
// Hosted.
func NewEventStore(
	path string,
) (Store, error) {
	store, err := events.New(path)
	if err != nil {
		return nil, err
	}
	return store, nil
}

// NewSnapshotStore returns a GORM-backed asynx snapshot store, opened against
// its own SQLite file rather than sharing the event store's.
func NewSnapshotStore(
	path string,
) (SnapshotStore, error) {
	store, err := snapshots.New(path)
	if err != nil {
		return nil, err
	}
	return store, nil
}

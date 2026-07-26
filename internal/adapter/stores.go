package adapter

import (
	asynxModels "github.com/char2cs/asynx/models"
)

// Stores pairs an aggregate's event stream with its snapshot cache. Both
// fields are interfaces, so this bundle leaks no concrete backend to its
// consumers.
type Stores struct {
	Events    asynxModels.Store
	Snapshots asynxModels.SnapshotStore
}

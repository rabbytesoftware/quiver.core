package mocks

import (
	"context"

	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge/internal/ports"
	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge/internal/store"
)

// ErrFindByOwnerReadModel is a test double that returns an error for FindByOwner.
type ErrFindByOwnerReadModel struct {
	store.PortStore
	Err error
}

func (e *ErrFindByOwnerReadModel) FindByOwner(
	_ context.Context,
	_ string,
) ([]ports.PortAllocation, error) {
	return nil, e.Err
}

package mocks

import (
	"context"

	"github.com/rabbytesoftware/quiver.core/internal/domain/netbridge"
)

type Netbridge struct {
	AllocatePort  int
	AllocateErr   error
	DeallocateErr error
}

func (m *Netbridge) Allocate(_ context.Context, _ string, _ netbridge.Protocol, _ int) (int, error) {
	return m.AllocatePort, m.AllocateErr
}

func (m *Netbridge) DeallocateByOwner(_ context.Context, _ string) error {
	return m.DeallocateErr
}

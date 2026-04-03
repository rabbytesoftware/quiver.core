package mocks

import (
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/ports"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/store"
)

// StubReadModel is a minimal test double for store.PortStore.
type StubReadModel struct {
	Data    map[int]*ports.PortAllocation
	FindErr error
}

// NewStubReadModel creates a new StubReadModel for testing.
func NewStubReadModel() *StubReadModel {
	return &StubReadModel{Data: make(map[int]*ports.PortAllocation)}
}

// Verify StubReadModel implements store.PortStore.
var _ store.PortStore = (*StubReadModel)(nil)

func (s *StubReadModel) Save(
	alloc ports.PortAllocation,
) error {
	s.Data[alloc.Port] = &alloc
	return nil
}

func (s *StubReadModel) Delete(
	port int,
) error {
	delete(s.Data, port)
	return nil
}

func (s *StubReadModel) FindByPort(
	port int,
) (*ports.PortAllocation, error) {
	if s.FindErr != nil {
		return nil, s.FindErr
	}
	return s.Data[port], nil
}

func (s *StubReadModel) FindByOwner(
	_ string,
) ([]ports.PortAllocation, error) {
	return nil, nil
}

func (s *StubReadModel) FindByID(
	id int,
) (*ports.PortAllocation, error) {
	if s.FindErr != nil {
		return nil, s.FindErr
	}
	return s.Data[id], nil
}

func (s *StubReadModel) FindAll() ([]ports.PortAllocation, error) {
	if s.FindErr != nil {
		return nil, s.FindErr
	}
	result := make([]ports.PortAllocation, 0, len(s.Data))
	for _, alloc := range s.Data {
		result = append(result, *alloc)
	}
	return result, nil
}

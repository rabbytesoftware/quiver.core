package mocks

import (
	"context"

	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge/internal/ports"
)

type StubReadModel struct {
	Data    map[int]*ports.PortAllocation
	FindErr error
}

func NewStubReadModel() *StubReadModel {
	return &StubReadModel{Data: make(map[int]*ports.PortAllocation)}
}

func (s *StubReadModel) Save(
	_ context.Context,
	alloc ports.PortAllocation,
) error {
	s.Data[alloc.Port] = &alloc
	return nil
}

func (s *StubReadModel) Delete(
	_ context.Context,
	port int,
) error {
	delete(s.Data, port)
	return nil
}

func (s *StubReadModel) FindByPort(
	_ context.Context,
	port int,
) (*ports.PortAllocation, error) {
	if s.FindErr != nil {
		return nil, s.FindErr
	}
	return s.Data[port], nil
}

func (s *StubReadModel) FindByOwner(
	_ context.Context,
	_ string,
) ([]ports.PortAllocation, error) {
	return nil, nil
}

func (s *StubReadModel) FindByKey(
	_ context.Context,
	id int,
) (*ports.PortAllocation, error) {
	if s.FindErr != nil {
		return nil, s.FindErr
	}
	return s.Data[id], nil
}

func (s *StubReadModel) FindAll(_ context.Context) ([]ports.PortAllocation, error) {
	if s.FindErr != nil {
		return nil, s.FindErr
	}
	result := make([]ports.PortAllocation, 0, len(s.Data))
	for _, alloc := range s.Data {
		result = append(result, *alloc)
	}
	return result, nil
}

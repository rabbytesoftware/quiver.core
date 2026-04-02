package mocks

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/ports"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/strategies"
)

// AlwaysAvailableStrategy is a test double that always succeeds at forwarding.
type AlwaysAvailableStrategy struct {
	ReverseCalledWith []int
}

// Verify AlwaysAvailableStrategy implements Strategy.
var _ strategies.Strategy = (*AlwaysAvailableStrategy)(nil)

func (s *AlwaysAvailableStrategy) Name() string {
	return "mock"
}

func (s *AlwaysAvailableStrategy) Available(
	_ context.Context,
) bool {
	return true
}

func (s *AlwaysAvailableStrategy) Forward(
	_ context.Context,
	_ int,
	_ ports.Protocol,
) error {
	return nil
}

func (s *AlwaysAvailableStrategy) Reverse(
	_ context.Context,
	port int,
	_ ports.Protocol,
) error {
	s.ReverseCalledWith = append(s.ReverseCalledWith, port)
	return nil
}

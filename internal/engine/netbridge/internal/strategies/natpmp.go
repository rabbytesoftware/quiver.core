package strategies

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/ports"
)

type natpmpStrategy struct{}

// NewNATPMP returns a NAT-PMP port-forwarding strategy.
func NewNATPMP() Strategy {
	return &natpmpStrategy{}
}

func (s *natpmpStrategy) Name() string {
	return "NAT-PMP"
}

func (s *natpmpStrategy) Available(
	_ context.Context,
) bool {
	// TODO: implement
	return false
}

func (s *natpmpStrategy) Forward(
	_ context.Context,
	_ int,
	_ ports.Protocol,
) error {
	// TODO: implement
	return nil
}

func (s *natpmpStrategy) Reverse(
	_ context.Context,
	_ int,
	_ ports.Protocol,
) error {
	// TODO: implement
	return nil
}

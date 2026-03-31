package strategies

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/ports"
)

type upnpStrategy struct{}

// NewUPnP returns a UPnP port-forwarding strategy.
func NewUPnP() Strategy {
	return &upnpStrategy{}
}

func (s *upnpStrategy) Name() string {
	return "UPnP"
}

func (s *upnpStrategy) Available(
	_ context.Context,
) bool {
	// TODO: implement
	return false
}

func (s *upnpStrategy) Forward(
	_ context.Context,
	_ int,
	_ ports.Protocol,
) error {
	// TODO: implement
	return nil
}

func (s *upnpStrategy) Reverse(
	_ context.Context,
	_ int,
	_ ports.Protocol,
) error {
	// TODO: implement
	return nil
}

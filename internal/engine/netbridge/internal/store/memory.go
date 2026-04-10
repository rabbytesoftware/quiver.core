package store

import (
	adaptermem "github.com/rabbytesoftware/quiver/internal/adapter/store/memory"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/ports"
)

func NewPortMemory() PortStore {
	return &portStore{
		inner: adaptermem.NewMemory[ports.PortAllocation, int](
			func(pa ports.PortAllocation) int { return pa.Port },
		),
	}
}

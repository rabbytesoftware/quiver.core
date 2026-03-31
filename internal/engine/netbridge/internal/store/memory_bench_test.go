package store

import (
	"fmt"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/ports"
)

func BenchmarkMemory_FindByOwner(
	b *testing.B,
) {
	rm := NewMemory()
	const owners = 10
	const perOwner = 100

	for i := 0; i < owners; i++ {
		for j := 0; j < perOwner; j++ {
			port := i*perOwner + j + 1000
			_ = rm.Save(ports.PortAllocation{
				Port:     port,
				OwnerKey: fmt.Sprintf("owner-%d", i),
				Protocol: ports.ProtocolTCP,
			})
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = rm.FindByOwner("owner-5")
	}
}

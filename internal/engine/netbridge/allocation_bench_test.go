package netbridge

import (
	"context"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/domain/netbridge"

	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/mocks"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/ports"
)

func BenchmarkFindAvailablePort_AllOccupied(
	b *testing.B,
) {
	const occupiedCount = 1000

	rm := mocks.NewStubReadModel()
	for port := testEphemeralPortStart; port < testEphemeralPortStart+occupiedCount; port++ {
		alloc := ports.PortAllocation{Port: port, OwnerKey: "bench-owner"}
		rm.Data[port] = &alloc
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = findAvailablePort(ctx, 0, testEphemeralPortStart, testEphemeralPortEnd, netbridge.ProtocolTCP, rm)
	}
}

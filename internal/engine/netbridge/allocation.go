package netbridge

import (
	"context"
	"fmt"
	"net"

	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/ports"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/store"
)

const (
	ephemeralPortStart = 49152
	ephemeralPortEnd   = 65535
)

func findAvailablePort(
	ctx context.Context,
	preferred int,
	protocol ports.Protocol,
	rm store.PortStore,
) (int, error) {
	if preferred != 0 && (preferred < 1 || preferred > 65535) {
		return 0, fmt.Errorf("%w: %d", ErrPortOutOfRange, preferred)
	}

	if preferred > 0 {
		ok, err := isPortAvailable(ctx, preferred, protocol, rm)
		if err != nil {
			return 0, err
		}
		if ok {
			return preferred, nil
		}
	}

	for candidate := ephemeralPortStart; candidate <= ephemeralPortEnd; candidate++ {
		ok, err := isPortAvailable(ctx, candidate, protocol, rm)
		if err != nil {
			return 0, err
		}
		if ok {
			return candidate, nil
		}
	}

	return 0, ErrNoPortAvailable
}

func isPortAvailable(
	ctx context.Context,
	port int,
	protocol ports.Protocol,
	rm store.PortStore,
) (bool, error) {
	alloc, err := rm.FindByPort(port)
	if err != nil {
		return false, err
	}
	if alloc != nil {
		return false, nil
	}
	return osBindTest(ctx, port, protocol)
}

func osBindTest(
	_ context.Context,
	port int,
	protocol ports.Protocol,
) (bool, error) {
	addr := fmt.Sprintf(":%d", port)

	if protocol == ports.ProtocolTCP || protocol == ports.ProtocolTCPUDP {
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return false, nil
		}
		ln.Close()
	}

	if protocol == ports.ProtocolUDP || protocol == ports.ProtocolTCPUDP {
		pc, err := net.ListenPacket("udp", addr)
		if err != nil {
			return false, nil
		}
		pc.Close()
	}

	return true, nil
}

package netbridge

import (
	"context"
	"fmt"
	"net"

	"github.com/rabbytesoftware/quiver/internal/domain/netbridge"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/store"
)

func findAvailablePort(
	ctx context.Context,
	preferred int,
	portStart int,
	portEnd int,
	protocol netbridge.Protocol,
	rm store.PortStore,
) (int, error) {
	if preferred != 0 && (preferred < 1 || preferred > 65535) {
		return 0, fmt.Errorf("%w: %d", ErrPortOutOfRange, preferred)
	}

	if preferred > 0 { //nolint:nestif
		ok, err := isPortAvailable(ctx, preferred, protocol, rm)
		if err != nil {
			return 0, err
		}
		if ok {
			return preferred, nil
		}
	}

	for candidate := portStart; candidate <= portEnd; candidate++ {
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
	protocol netbridge.Protocol,
	rm store.PortStore,
) (bool, error) {
	alloc, err := rm.FindByPort(ctx, port)
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
	protocol netbridge.Protocol,
) (bool, error) {
	addr := fmt.Sprintf(":%d", port)

	if protocol == netbridge.ProtocolTCP || protocol == netbridge.ProtocolTCPUDP {
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return false, nil
		}
		_ = ln.Close()
	}

	if protocol == netbridge.ProtocolUDP || protocol == netbridge.ProtocolTCPUDP {
		pc, err := net.ListenPacket("udp", addr)
		if err != nil {
			return false, nil
		}
		_ = pc.Close()
	}

	return true, nil
}

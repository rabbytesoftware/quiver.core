package netbridge

import (
	"context"
	"errors"
	"github.com/rabbytesoftware/quiver/internal/domain/netbridge"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/mocks"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/ports"
)

const (
	testEphemeralPortStart = 49152
	testEphemeralPortEnd   = 65535
)

func TestFindAvailablePort_PreferredAvailable(
	t *testing.T,
) {
	rm := mocks.NewStubReadModel()
	port, err := findAvailablePort(context.Background(), 54321, testEphemeralPortStart, testEphemeralPortEnd, netbridge.ProtocolTCP, rm)
	require.NoError(t, err)
	assert.Equal(t, 54321, port)
}

func TestFindAvailablePort_PreferredZero(
	t *testing.T,
) {
	rm := mocks.NewStubReadModel()
	port, err := findAvailablePort(context.Background(), 0, testEphemeralPortStart, testEphemeralPortEnd, netbridge.ProtocolTCP, rm)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, port, testEphemeralPortStart)
	assert.LessOrEqual(t, port, testEphemeralPortEnd)
}

func TestFindAvailablePort_OutOfRange_Negative(
	t *testing.T,
) {
	rm := mocks.NewStubReadModel()
	_, err := findAvailablePort(context.Background(), -1, testEphemeralPortStart, testEphemeralPortEnd, netbridge.ProtocolTCP, rm)
	assert.ErrorIs(t, err, ErrPortOutOfRange)
}

func TestFindAvailablePort_OutOfRange_TooHigh(
	t *testing.T,
) {
	rm := mocks.NewStubReadModel()
	_, err := findAvailablePort(context.Background(), 99999, testEphemeralPortStart, testEphemeralPortEnd, netbridge.ProtocolTCP, rm)
	assert.ErrorIs(t, err, ErrPortOutOfRange)
}

func TestFindAvailablePort_PreferredTaken_FallsBack(
	t *testing.T,
) {
	rm := mocks.NewStubReadModel()
	taken := ports.PortAllocation{Port: 54321, OwnerKey: "other"}
	rm.Data[54321] = &taken

	port, err := findAvailablePort(context.Background(), 54321, testEphemeralPortStart, testEphemeralPortEnd, netbridge.ProtocolTCP, rm)
	require.NoError(t, err)
	assert.NotEqual(t, 54321, port)
	assert.GreaterOrEqual(t, port, testEphemeralPortStart)
}

func TestFindAvailablePort_PreferredScanError(
	t *testing.T,
) {
	rm := mocks.NewStubReadModel()
	rm.FindErr = errors.New("storage error")

	_, err := findAvailablePort(context.Background(), 54321, testEphemeralPortStart, testEphemeralPortEnd, netbridge.ProtocolTCP, rm)
	assert.Error(t, err)
}

func TestFindAvailablePort_EphemeralScanError(
	t *testing.T,
) {
	rm := mocks.NewStubReadModel()
	rm.FindErr = errors.New("storage error")

	_, err := findAvailablePort(context.Background(), 0, testEphemeralPortStart, testEphemeralPortEnd, netbridge.ProtocolTCP, rm)
	assert.Error(t, err)
}

func TestIsPortAvailable_ReadModelError(
	t *testing.T,
) {
	rm := mocks.NewStubReadModel()
	rm.FindErr = errors.New("storage error")

	_, err := isPortAvailable(context.Background(), 8080, netbridge.ProtocolTCP, rm)
	assert.Error(t, err)
}

func TestIsPortAvailable_AlreadyTracked(
	t *testing.T,
) {
	rm := mocks.NewStubReadModel()
	alloc := ports.PortAllocation{Port: 8080}
	rm.Data[8080] = &alloc

	ok, err := isPortAvailable(context.Background(), 8080, netbridge.ProtocolTCP, rm)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestOsBindTest_TCP(
	t *testing.T,
) {
	rm := mocks.NewStubReadModel()
	port, err := findAvailablePort(context.Background(), 0, testEphemeralPortStart, testEphemeralPortEnd, netbridge.ProtocolTCP, rm)
	require.NoError(t, err)

	ok, err := osBindTest(context.Background(), port, netbridge.ProtocolTCP)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestOsBindTest_UDP(
	t *testing.T,
) {
	rm := mocks.NewStubReadModel()
	port, err := findAvailablePort(context.Background(), 0, testEphemeralPortStart, testEphemeralPortEnd, netbridge.ProtocolUDP, rm)
	require.NoError(t, err)

	ok, err := osBindTest(context.Background(), port, netbridge.ProtocolUDP)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestOsBindTest_TCPUDP(
	t *testing.T,
) {
	rm := mocks.NewStubReadModel()
	port, err := findAvailablePort(context.Background(), 0, testEphemeralPortStart, testEphemeralPortEnd, netbridge.ProtocolTCPUDP, rm)
	require.NoError(t, err)

	ok, err := osBindTest(context.Background(), port, netbridge.ProtocolTCPUDP)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestOsBindTest_BoundTCPPort(
	t *testing.T,
) {
	ln, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	defer ln.Close()

	addr := ln.Addr().(*net.TCPAddr)
	ok, bindErr := osBindTest(context.Background(), addr.Port, netbridge.ProtocolTCP)
	require.NoError(t, bindErr)
	assert.False(t, ok)
}

func TestOsBindTest_BoundUDPPort(
	t *testing.T,
) {
	pc, err := net.ListenPacket("udp", ":0")
	require.NoError(t, err)
	defer pc.Close()

	addr := pc.LocalAddr().(*net.UDPAddr)
	ok, bindErr := osBindTest(context.Background(), addr.Port, netbridge.ProtocolUDP)
	require.NoError(t, bindErr)
	assert.False(t, ok)
}

func TestFindAvailablePort_ErrNoPortAvailable(
	t *testing.T,
) {
	rm := mocks.NewStubReadModel()
	for port := testEphemeralPortStart; port <= testEphemeralPortEnd; port++ {
		alloc := ports.PortAllocation{Port: port, OwnerKey: "owner"}
		rm.Data[port] = &alloc
	}

	_, err := findAvailablePort(context.Background(), 0, testEphemeralPortStart, testEphemeralPortEnd, netbridge.ProtocolTCP, rm)
	assert.ErrorIs(t, err, ErrNoPortAvailable)
}

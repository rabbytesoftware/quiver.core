package netbridge

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/ports"
)

// stubReadModel is a minimal readmodel.Store for allocation unit tests.
type stubReadModel struct {
	data    map[int]*ports.PortAllocation
	findErr error
}

func newStubReadModel() *stubReadModel {
	return &stubReadModel{data: make(map[int]*ports.PortAllocation)}
}

func (s *stubReadModel) Save(
	alloc ports.PortAllocation,
) error {
	s.data[alloc.Port] = &alloc
	return nil
}

func (s *stubReadModel) Delete(
	port int,
) error {
	delete(s.data, port)
	return nil
}

func (s *stubReadModel) FindByPort(
	port int,
) (*ports.PortAllocation, error) {
	if s.findErr != nil {
		return nil, s.findErr
	}
	return s.data[port], nil
}

func (s *stubReadModel) FindByOwner(
	_ string,
) ([]ports.PortAllocation, error) {
	return nil, nil
}

func TestFindAvailablePort_PreferredAvailable(
	t *testing.T,
) {
	rm := newStubReadModel()
	port, err := findAvailablePort(context.Background(), 54321, ports.ProtocolTCP, rm)
	require.NoError(t, err)
	assert.Equal(t, 54321, port)
}

func TestFindAvailablePort_PreferredZero(
	t *testing.T,
) {
	rm := newStubReadModel()
	port, err := findAvailablePort(context.Background(), 0, ports.ProtocolTCP, rm)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, port, ephemeralPortStart)
	assert.LessOrEqual(t, port, ephemeralPortEnd)
}

func TestFindAvailablePort_OutOfRange_Negative(
	t *testing.T,
) {
	rm := newStubReadModel()
	_, err := findAvailablePort(context.Background(), -1, ports.ProtocolTCP, rm)
	assert.ErrorIs(t, err, ErrPortOutOfRange)
}

func TestFindAvailablePort_OutOfRange_TooHigh(
	t *testing.T,
) {
	rm := newStubReadModel()
	_, err := findAvailablePort(context.Background(), 99999, ports.ProtocolTCP, rm)
	assert.ErrorIs(t, err, ErrPortOutOfRange)
}

func TestFindAvailablePort_PreferredTaken_FallsBack(
	t *testing.T,
) {
	rm := newStubReadModel()
	taken := ports.PortAllocation{Port: 54321, OwnerKey: "other"}
	rm.data[54321] = &taken

	port, err := findAvailablePort(context.Background(), 54321, ports.ProtocolTCP, rm)
	require.NoError(t, err)
	assert.NotEqual(t, 54321, port)
	assert.GreaterOrEqual(t, port, ephemeralPortStart)
}

func TestFindAvailablePort_PreferredScanError(
	t *testing.T,
) {
	rm := newStubReadModel()
	rm.findErr = errors.New("storage error")

	_, err := findAvailablePort(context.Background(), 54321, ports.ProtocolTCP, rm)
	assert.Error(t, err)
}

func TestFindAvailablePort_EphemeralScanError(
	t *testing.T,
) {
	rm := newStubReadModel()
	rm.findErr = errors.New("storage error")

	_, err := findAvailablePort(context.Background(), 0, ports.ProtocolTCP, rm)
	assert.Error(t, err)
}

func TestIsPortAvailable_ReadModelError(
	t *testing.T,
) {
	rm := newStubReadModel()
	rm.findErr = errors.New("storage error")

	_, err := isPortAvailable(context.Background(), 8080, ports.ProtocolTCP, rm)
	assert.Error(t, err)
}

func TestIsPortAvailable_AlreadyTracked(
	t *testing.T,
) {
	rm := newStubReadModel()
	alloc := ports.PortAllocation{Port: 8080}
	rm.data[8080] = &alloc

	ok, err := isPortAvailable(context.Background(), 8080, ports.ProtocolTCP, rm)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestOsBindTest_TCP(
	t *testing.T,
) {
	rm := newStubReadModel()
	port, err := findAvailablePort(context.Background(), 0, ports.ProtocolTCP, rm)
	require.NoError(t, err)

	ok, err := osBindTest(context.Background(), port, ports.ProtocolTCP)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestOsBindTest_UDP(
	t *testing.T,
) {
	rm := newStubReadModel()
	port, err := findAvailablePort(context.Background(), 0, ports.ProtocolUDP, rm)
	require.NoError(t, err)

	ok, err := osBindTest(context.Background(), port, ports.ProtocolUDP)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestOsBindTest_TCPUDP(
	t *testing.T,
) {
	rm := newStubReadModel()
	port, err := findAvailablePort(context.Background(), 0, ports.ProtocolTCPUDP, rm)
	require.NoError(t, err)

	ok, err := osBindTest(context.Background(), port, ports.ProtocolTCPUDP)
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
	ok, bindErr := osBindTest(context.Background(), addr.Port, ports.ProtocolTCP)
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
	ok, bindErr := osBindTest(context.Background(), addr.Port, ports.ProtocolUDP)
	require.NoError(t, bindErr)
	assert.False(t, ok)
}

func TestFindAvailablePort_ErrNoPortAvailable(
	t *testing.T,
) {
	rm := newStubReadModel()
	for port := ephemeralPortStart; port <= ephemeralPortEnd; port++ {
		alloc := ports.PortAllocation{Port: port, OwnerKey: "owner"}
		rm.data[port] = &alloc
	}

	_, err := findAvailablePort(context.Background(), 0, ports.ProtocolTCP, rm)
	assert.ErrorIs(t, err, ErrNoPortAvailable)
}

package strategies

import (
	"context"
	"errors"
	"net"
	"testing"

	natpmp "github.com/jackpal/go-nat-pmp"
	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver/internal/domain/netbridge"
)

func TestNATPMP_NewNATPMP(
	t *testing.T,
) {
	s := NewNATPMP()
	assert.NotNil(t, s)
	assert.Equal(t, "NAT-PMP", s.Name())
}

func TestNATPMP_Constructor_RealDeps(
	t *testing.T,
) {
	s := NewNATPMP()
	assert.NotNil(t, s)
	strategy, ok := s.(*natpmpStrategy)
	assert.True(t, ok)
	assert.NotNil(t, strategy.gateway)
	assert.NotNil(t, strategy.newClient)
}

func TestNATPMP_NewClient_Function(
	t *testing.T,
) {
	// Test that natpmpNewClient correctly wraps the library call
	// (will panic on invalid IP, but that's OK - testing the wrapper works)
	gw := net.ParseIP("192.168.1.1")
	client := natpmpNewClient(gw)
	assert.NotNil(t, client)
}

func TestNATPMP_Name(
	t *testing.T,
) {
	s := NewNATPMP()
	assert.Equal(t, "NAT-PMP", s.Name())
}

func TestNATPMP_Available_NoGateway(
	t *testing.T,
) {
	s := &natpmpStrategy{
		gateway: func() (net.IP, error) {
			return nil, errors.New("gateway not found")
		},
	}

	assert.False(t, s.Available(context.Background()))
}

func TestNATPMP_Available_ClientError(
	t *testing.T,
) {
	mockClient := &mockNATPMPClient{
		getExternalAddressErr: errors.New("client error"),
	}

	s := &natpmpStrategy{
		gateway: func() (net.IP, error) {
			return net.ParseIP("192.168.1.1"), nil
		},
		newClient: func(net.IP) natpmpClient {
			return mockClient
		},
	}

	assert.False(t, s.Available(context.Background()))
}

func TestNATPMP_Available_Success(
	t *testing.T,
) {
	mockClient := &mockNATPMPClient{}

	s := &natpmpStrategy{
		gateway: func() (net.IP, error) {
			return net.ParseIP("192.168.1.1"), nil
		},
		newClient: func(net.IP) natpmpClient {
			return mockClient
		},
	}

	assert.True(t, s.Available(context.Background()))
}

func TestNATPMP_Forward_NoGateway(
	t *testing.T,
) {
	s := &natpmpStrategy{
		gateway: func() (net.IP, error) {
			return nil, errors.New("gateway not found")
		},
	}

	err := s.Forward(context.Background(), 8080, netbridge.ProtocolTCP)
	assert.Error(t, err)
}

func TestNATPMP_Forward_TCP(
	t *testing.T,
) {
	mockClient := &mockNATPMPClient{}

	s := &natpmpStrategy{
		gateway: func() (net.IP, error) {
			return net.ParseIP("192.168.1.1"), nil
		},
		newClient: func(net.IP) natpmpClient {
			return mockClient
		},
	}

	err := s.Forward(context.Background(), 8080, netbridge.ProtocolTCP)
	assert.NoError(t, err)
	assert.Len(t, mockClient.addPortMappingCalls, 1)
	assert.Equal(t, "tcp", mockClient.addPortMappingCalls[0].protocol)
	assert.Equal(t, 8080, mockClient.addPortMappingCalls[0].internalPort)
	assert.Equal(t, 3600, mockClient.addPortMappingCalls[0].lifetime)
}

func TestNATPMP_Forward_UDP(
	t *testing.T,
) {
	mockClient := &mockNATPMPClient{}

	s := &natpmpStrategy{
		gateway: func() (net.IP, error) {
			return net.ParseIP("192.168.1.1"), nil
		},
		newClient: func(net.IP) natpmpClient {
			return mockClient
		},
	}

	err := s.Forward(context.Background(), 9090, netbridge.ProtocolUDP)
	assert.NoError(t, err)
	assert.Len(t, mockClient.addPortMappingCalls, 1)
	assert.Equal(t, "udp", mockClient.addPortMappingCalls[0].protocol)
	assert.Equal(t, 9090, mockClient.addPortMappingCalls[0].internalPort)
}

func TestNATPMP_Forward_TCPUDP(
	t *testing.T,
) {
	mockClient := &mockNATPMPClient{}

	s := &natpmpStrategy{
		gateway: func() (net.IP, error) {
			return net.ParseIP("192.168.1.1"), nil
		},
		newClient: func(net.IP) natpmpClient {
			return mockClient
		},
	}

	err := s.Forward(context.Background(), 8080, netbridge.ProtocolTCPUDP)
	assert.NoError(t, err)
	assert.Len(t, mockClient.addPortMappingCalls, 2)
	assert.Equal(t, "tcp", mockClient.addPortMappingCalls[0].protocol)
	assert.Equal(t, "udp", mockClient.addPortMappingCalls[1].protocol)
}

func TestNATPMP_Forward_ClientError(
	t *testing.T,
) {
	mockClient := &mockNATPMPClient{
		addPortMappingErr: errors.New("port already in use"),
	}

	s := &natpmpStrategy{
		gateway: func() (net.IP, error) {
			return net.ParseIP("192.168.1.1"), nil
		},
		newClient: func(net.IP) natpmpClient {
			return mockClient
		},
	}

	err := s.Forward(context.Background(), 8080, netbridge.ProtocolTCP)
	assert.Error(t, err)
}

func TestNATPMP_Forward_TCPUDP_SecondFails(
	t *testing.T,
) {
	callCount := 0
	mockClient := &mockNATPMPClient{
		addPortMappingFn: func() error {
			callCount++
			if callCount == 2 {
				return errors.New("second call failed")
			}
			return nil
		},
	}

	s := &natpmpStrategy{
		gateway: func() (net.IP, error) {
			return net.ParseIP("192.168.1.1"), nil
		},
		newClient: func(net.IP) natpmpClient {
			return mockClient
		},
	}

	err := s.Forward(context.Background(), 8080, netbridge.ProtocolTCPUDP)
	assert.Error(t, err)
}

func TestNATPMP_Reverse_NoGateway(
	t *testing.T,
) {
	s := &natpmpStrategy{
		gateway: func() (net.IP, error) {
			return nil, errors.New("gateway not found")
		},
	}

	err := s.Reverse(context.Background(), 8080, netbridge.ProtocolTCP)
	assert.Error(t, err)
}

func TestNATPMP_Reverse_TCP(
	t *testing.T,
) {
	mockClient := &mockNATPMPClient{}

	s := &natpmpStrategy{
		gateway: func() (net.IP, error) {
			return net.ParseIP("192.168.1.1"), nil
		},
		newClient: func(net.IP) natpmpClient {
			return mockClient
		},
	}

	err := s.Reverse(context.Background(), 8080, netbridge.ProtocolTCP)
	assert.NoError(t, err)
	assert.Len(t, mockClient.addPortMappingCalls, 1)
	assert.Equal(t, "tcp", mockClient.addPortMappingCalls[0].protocol)
	assert.Equal(t, 0, mockClient.addPortMappingCalls[0].lifetime)
}

func TestNATPMP_Reverse_UDP(
	t *testing.T,
) {
	mockClient := &mockNATPMPClient{}

	s := &natpmpStrategy{
		gateway: func() (net.IP, error) {
			return net.ParseIP("192.168.1.1"), nil
		},
		newClient: func(net.IP) natpmpClient {
			return mockClient
		},
	}

	err := s.Reverse(context.Background(), 9090, netbridge.ProtocolUDP)
	assert.NoError(t, err)
	assert.Len(t, mockClient.addPortMappingCalls, 1)
	assert.Equal(t, "udp", mockClient.addPortMappingCalls[0].protocol)
	assert.Equal(t, 0, mockClient.addPortMappingCalls[0].lifetime)
}

func TestNATPMP_Reverse_TCPUDP(
	t *testing.T,
) {
	mockClient := &mockNATPMPClient{}

	s := &natpmpStrategy{
		gateway: func() (net.IP, error) {
			return net.ParseIP("192.168.1.1"), nil
		},
		newClient: func(net.IP) natpmpClient {
			return mockClient
		},
	}

	err := s.Reverse(context.Background(), 8080, netbridge.ProtocolTCPUDP)
	assert.NoError(t, err)
	assert.Len(t, mockClient.addPortMappingCalls, 2)
	assert.Equal(t, 0, mockClient.addPortMappingCalls[0].lifetime)
	assert.Equal(t, 0, mockClient.addPortMappingCalls[1].lifetime)
}

func TestNATPMP_Reverse_ClientError(
	t *testing.T,
) {
	mockClient := &mockNATPMPClient{
		addPortMappingErr: errors.New("failed to reverse"),
	}

	s := &natpmpStrategy{
		gateway: func() (net.IP, error) {
			return net.ParseIP("192.168.1.1"), nil
		},
		newClient: func(net.IP) natpmpClient {
			return mockClient
		},
	}

	err := s.Reverse(context.Background(), 8080, netbridge.ProtocolTCP)
	assert.Error(t, err)
}

func TestNATPMP_Protocols_TCP(
	t *testing.T,
) {
	result := natpmpProtocols(netbridge.ProtocolTCP)
	assert.Equal(t, []string{"tcp"}, result)
}

func TestNATPMP_Protocols_UDP(
	t *testing.T,
) {
	result := natpmpProtocols(netbridge.ProtocolUDP)
	assert.Equal(t, []string{"udp"}, result)
}

func TestNATPMP_Protocols_TCPUDP(
	t *testing.T,
) {
	result := natpmpProtocols(netbridge.ProtocolTCPUDP)
	assert.Equal(t, []string{"tcp", "udp"}, result)
}

func TestNATPMP_Protocols_Unknown(
	t *testing.T,
) {
	result := natpmpProtocols(netbridge.Protocol("unknown"))
	assert.Equal(t, []string{}, result)
}

type addPortMappingCall struct {
	protocol     string
	internalPort int
	externalPort int
	lifetime     int
}

type mockNATPMPClient struct {
	getExternalAddressErr error
	addPortMappingErr     error
	addPortMappingFn      func() error
	addPortMappingCalls   []addPortMappingCall
}

func (m *mockNATPMPClient) GetExternalAddress() (*natpmp.GetExternalAddressResult, error) {
	if m.getExternalAddressErr != nil {
		return nil, m.getExternalAddressErr
	}
	return &natpmp.GetExternalAddressResult{}, nil
}

func (m *mockNATPMPClient) AddPortMapping(
	protocol string,
	internalPort int,
	externalPort int,
	lifetime int,
) (*natpmp.AddPortMappingResult, error) {
	if m.addPortMappingFn != nil {
		err := m.addPortMappingFn()
		if err != nil {
			return nil, err
		}
	}

	if m.addPortMappingErr != nil {
		return nil, m.addPortMappingErr
	}

	m.addPortMappingCalls = append(m.addPortMappingCalls, addPortMappingCall{
		protocol:     protocol,
		internalPort: internalPort,
		externalPort: externalPort,
		lifetime:     lifetime,
	})

	return &natpmp.AddPortMappingResult{}, nil
}

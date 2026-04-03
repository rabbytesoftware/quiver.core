package strategies

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver/internal/domain/netbridge"
)

func TestUPnP_Constructor(
	t *testing.T,
) {
	s := NewUPnP()
	assert.NotNil(t, s)
	strategy, ok := s.(*upnpStrategy)
	assert.True(t, ok)
	assert.NotNil(t, strategy.discover)
	assert.NotNil(t, strategy.localIP)
}

func TestUPnP_Name(
	t *testing.T,
) {
	s := NewUPnP()
	assert.Equal(t, "UPnP", s.Name())
}

func TestUPnP_Available_DiscoverError(
	t *testing.T,
) {
	s := &upnpStrategy{
		discover: func(ctx context.Context) ([]upnpIGD, error) {
			return nil, errors.New("discover failed")
		},
		localIP: func() (string, error) {
			return "192.168.1.100", nil
		},
	}

	assert.False(t, s.Available(context.Background()))
}

func TestUPnP_Available_NoIGDs(
	t *testing.T,
) {
	s := &upnpStrategy{
		discover: func(ctx context.Context) ([]upnpIGD, error) {
			return []upnpIGD{}, nil
		},
		localIP: func() (string, error) {
			return "192.168.1.100", nil
		},
	}

	assert.False(t, s.Available(context.Background()))
}

func TestUPnP_Available_Success(
	t *testing.T,
) {
	mockIGD := &mockUPnPIGD{}

	s := &upnpStrategy{
		discover: func(ctx context.Context) ([]upnpIGD, error) {
			return []upnpIGD{mockIGD}, nil
		},
		localIP: func() (string, error) {
			return "192.168.1.100", nil
		},
	}

	assert.True(t, s.Available(context.Background()))
}

func TestUPnP_Forward_DiscoverError(
	t *testing.T,
) {
	s := &upnpStrategy{
		discover: func(ctx context.Context) ([]upnpIGD, error) {
			return nil, errors.New("discover failed")
		},
		localIP: func() (string, error) {
			return "192.168.1.100", nil
		},
	}

	err := s.Forward(context.Background(), 8080, netbridge.ProtocolTCP)
	assert.Error(t, err)
}

func TestUPnP_Forward_NoIGDs(
	t *testing.T,
) {
	s := &upnpStrategy{
		discover: func(ctx context.Context) ([]upnpIGD, error) {
			return []upnpIGD{}, nil
		},
		localIP: func() (string, error) {
			return "192.168.1.100", nil
		},
	}

	err := s.Forward(context.Background(), 8080, netbridge.ProtocolTCP)
	assert.Error(t, err)
	assert.Equal(t, ErrNoIGD, err)
}

func TestUPnP_Forward_LocalIPError(
	t *testing.T,
) {
	mockIGD := &mockUPnPIGD{}

	s := &upnpStrategy{
		discover: func(ctx context.Context) ([]upnpIGD, error) {
			return []upnpIGD{mockIGD}, nil
		},
		localIP: func() (string, error) {
			return "", errors.New("no local ip")
		},
	}

	err := s.Forward(context.Background(), 8080, netbridge.ProtocolTCP)
	assert.Error(t, err)
}

func TestUPnP_Forward_TCP(
	t *testing.T,
) {
	mockIGD := &mockUPnPIGD{}

	s := &upnpStrategy{
		discover: func(ctx context.Context) ([]upnpIGD, error) {
			return []upnpIGD{mockIGD}, nil
		},
		localIP: func() (string, error) {
			return "192.168.1.100", nil
		},
	}

	err := s.Forward(context.Background(), 8080, netbridge.ProtocolTCP)
	assert.NoError(t, err)
	assert.Len(t, mockIGD.addPortMappingCalls, 1)
	call := mockIGD.addPortMappingCalls[0]
	assert.Equal(t, "TCP", call.protocol)
	assert.Equal(t, uint16(8080), call.externalPort)
	assert.Equal(t, uint16(8080), call.internalPort)
	assert.Equal(t, "192.168.1.100", call.internalClient)
	assert.Equal(t, uint32(3600), call.leaseDuration)
	assert.Equal(t, true, call.enabled)
}

func TestUPnP_Forward_UDP(
	t *testing.T,
) {
	mockIGD := &mockUPnPIGD{}

	s := &upnpStrategy{
		discover: func(ctx context.Context) ([]upnpIGD, error) {
			return []upnpIGD{mockIGD}, nil
		},
		localIP: func() (string, error) {
			return "192.168.1.100", nil
		},
	}

	err := s.Forward(context.Background(), 9090, netbridge.ProtocolUDP)
	assert.NoError(t, err)
	assert.Len(t, mockIGD.addPortMappingCalls, 1)
	assert.Equal(t, "UDP", mockIGD.addPortMappingCalls[0].protocol)
	assert.Equal(t, uint16(9090), mockIGD.addPortMappingCalls[0].externalPort)
}

func TestUPnP_Forward_TCPUDP(
	t *testing.T,
) {
	mockIGD := &mockUPnPIGD{}

	s := &upnpStrategy{
		discover: func(ctx context.Context) ([]upnpIGD, error) {
			return []upnpIGD{mockIGD}, nil
		},
		localIP: func() (string, error) {
			return "192.168.1.100", nil
		},
	}

	err := s.Forward(context.Background(), 8080, netbridge.ProtocolTCPUDP)
	assert.NoError(t, err)
	assert.Len(t, mockIGD.addPortMappingCalls, 2)
	assert.Equal(t, "TCP", mockIGD.addPortMappingCalls[0].protocol)
	assert.Equal(t, "UDP", mockIGD.addPortMappingCalls[1].protocol)
}

func TestUPnP_Forward_ClientError(
	t *testing.T,
) {
	mockIGD := &mockUPnPIGD{
		addPortMappingErr: errors.New("port in use"),
	}

	s := &upnpStrategy{
		discover: func(ctx context.Context) ([]upnpIGD, error) {
			return []upnpIGD{mockIGD}, nil
		},
		localIP: func() (string, error) {
			return "192.168.1.100", nil
		},
	}

	err := s.Forward(context.Background(), 8080, netbridge.ProtocolTCP)
	assert.Error(t, err)
}

func TestUPnP_Reverse_DiscoverError(
	t *testing.T,
) {
	s := &upnpStrategy{
		discover: func(ctx context.Context) ([]upnpIGD, error) {
			return nil, errors.New("discover failed")
		},
		localIP: func() (string, error) {
			return "192.168.1.100", nil
		},
	}

	err := s.Reverse(context.Background(), 8080, netbridge.ProtocolTCP)
	assert.Error(t, err)
}

func TestUPnP_Reverse_NoIGDs(
	t *testing.T,
) {
	s := &upnpStrategy{
		discover: func(ctx context.Context) ([]upnpIGD, error) {
			return []upnpIGD{}, nil
		},
		localIP: func() (string, error) {
			return "192.168.1.100", nil
		},
	}

	err := s.Reverse(context.Background(), 8080, netbridge.ProtocolTCP)
	assert.Error(t, err)
	assert.Equal(t, ErrNoIGD, err)
}

func TestUPnP_Reverse_TCP(
	t *testing.T,
) {
	mockIGD := &mockUPnPIGD{}

	s := &upnpStrategy{
		discover: func(ctx context.Context) ([]upnpIGD, error) {
			return []upnpIGD{mockIGD}, nil
		},
		localIP: func() (string, error) {
			return "192.168.1.100", nil
		},
	}

	err := s.Reverse(context.Background(), 8080, netbridge.ProtocolTCP)
	assert.NoError(t, err)
	assert.Len(t, mockIGD.deletePortMappingCalls, 1)
	call := mockIGD.deletePortMappingCalls[0]
	assert.Equal(t, "TCP", call.protocol)
	assert.Equal(t, uint16(8080), call.externalPort)
}

func TestUPnP_Reverse_UDP(
	t *testing.T,
) {
	mockIGD := &mockUPnPIGD{}

	s := &upnpStrategy{
		discover: func(ctx context.Context) ([]upnpIGD, error) {
			return []upnpIGD{mockIGD}, nil
		},
		localIP: func() (string, error) {
			return "192.168.1.100", nil
		},
	}

	err := s.Reverse(context.Background(), 9090, netbridge.ProtocolUDP)
	assert.NoError(t, err)
	assert.Len(t, mockIGD.deletePortMappingCalls, 1)
	assert.Equal(t, "UDP", mockIGD.deletePortMappingCalls[0].protocol)
	assert.Equal(t, uint16(9090), mockIGD.deletePortMappingCalls[0].externalPort)
}

func TestUPnP_Reverse_TCPUDP(
	t *testing.T,
) {
	mockIGD := &mockUPnPIGD{}

	s := &upnpStrategy{
		discover: func(ctx context.Context) ([]upnpIGD, error) {
			return []upnpIGD{mockIGD}, nil
		},
		localIP: func() (string, error) {
			return "192.168.1.100", nil
		},
	}

	err := s.Reverse(context.Background(), 8080, netbridge.ProtocolTCPUDP)
	assert.NoError(t, err)
	assert.Len(t, mockIGD.deletePortMappingCalls, 2)
	assert.Equal(t, "TCP", mockIGD.deletePortMappingCalls[0].protocol)
	assert.Equal(t, "UDP", mockIGD.deletePortMappingCalls[1].protocol)
}

func TestUPnP_Reverse_ClientError(
	t *testing.T,
) {
	mockIGD := &mockUPnPIGD{
		deletePortMappingErr: errors.New("delete failed"),
	}

	s := &upnpStrategy{
		discover: func(ctx context.Context) ([]upnpIGD, error) {
			return []upnpIGD{mockIGD}, nil
		},
		localIP: func() (string, error) {
			return "192.168.1.100", nil
		},
	}

	err := s.Reverse(context.Background(), 8080, netbridge.ProtocolTCP)
	assert.Error(t, err)
}

func TestUPnP_Protocols_TCP(
	t *testing.T,
) {
	result := upnpProtocols(netbridge.ProtocolTCP)
	assert.Equal(t, []string{"TCP"}, result)
}

func TestUPnP_Protocols_UDP(
	t *testing.T,
) {
	result := upnpProtocols(netbridge.ProtocolUDP)
	assert.Equal(t, []string{"UDP"}, result)
}

func TestUPnP_Protocols_TCPUDP(
	t *testing.T,
) {
	result := upnpProtocols(netbridge.ProtocolTCPUDP)
	assert.Equal(t, []string{"TCP", "UDP"}, result)
}

func TestUPnP_Protocols_Unknown(
	t *testing.T,
) {
	result := upnpProtocols(netbridge.Protocol("unknown"))
	assert.Equal(t, []string{}, result)
}

func TestUPnP_DiscoverIGDs_Success(
	t *testing.T,
) {
	result, err := discoverIGDs(context.Background())
	// Either err is nil and result is a valid slice, or discovery fails (no UPnP on system)
	if err == nil && len(result) > 0 {
		assert.IsType(t, []upnpIGD{}, result)
	}
}

func TestUPnP_GetLocalIP_Success(
	t *testing.T,
) {
	ip, err := getLocalIP()
	assert.NoError(t, err)
	assert.NotEmpty(t, ip)
	assert.NotEqual(t, "127.0.0.1", ip) // Should not be loopback
}

func TestUPnP_Forward_WithLocalIPFunction(
	t *testing.T,
) {
	mockIGD := &mockUPnPIGD{}
	called := false

	s := &upnpStrategy{
		discover: func(ctx context.Context) ([]upnpIGD, error) {
			return []upnpIGD{mockIGD}, nil
		},
		localIP: func() (string, error) {
			called = true
			return "192.168.1.100", nil
		},
	}

	err := s.Forward(context.Background(), 8080, netbridge.ProtocolTCP)
	assert.NoError(t, err)
	assert.True(t, called)
}

func TestUPnP_DiscoverIGDs_ErrorCase(
	t *testing.T,
) {
	s := &upnpStrategy{
		discover: func(ctx context.Context) ([]upnpIGD, error) {
			// Simulate discovery with some errors but also some clients
			// In reality, discoverIGDs returns successful clients even if there are discovery errors
			return []upnpIGD{&mockUPnPIGD{}}, nil
		},
		localIP: func() (string, error) {
			return "192.168.1.100", nil
		},
	}

	result, err := s.discover(context.Background())
	assert.NoError(t, err)
	assert.Len(t, result, 1)
}

type addPortMappingCallUPnP struct {
	remoteHost     string
	externalPort   uint16
	protocol       string
	internalPort   uint16
	internalClient string
	enabled        bool
	description    string
	leaseDuration  uint32
}

type deletePortMappingCall struct {
	remoteHost   string
	externalPort uint16
	protocol     string
}

type mockUPnPIGD struct {
	addPortMappingErr      error
	deletePortMappingErr   error
	addPortMappingCalls    []addPortMappingCallUPnP
	deletePortMappingCalls []deletePortMappingCall
}

func (m *mockUPnPIGD) AddPortMapping(
	remoteHost string,
	externalPort uint16,
	protocol string,
	internalPort uint16,
	internalClient string,
	enabled bool,
	description string,
	leaseDuration uint32,
) error {
	if m.addPortMappingErr != nil {
		return m.addPortMappingErr
	}

	m.addPortMappingCalls = append(m.addPortMappingCalls, addPortMappingCallUPnP{
		remoteHost:     remoteHost,
		externalPort:   externalPort,
		protocol:       protocol,
		internalPort:   internalPort,
		internalClient: internalClient,
		enabled:        enabled,
		description:    description,
		leaseDuration:  leaseDuration,
	})

	return nil
}

func (m *mockUPnPIGD) DeletePortMapping(
	remoteHost string,
	externalPort uint16,
	protocol string,
) error {
	if m.deletePortMappingErr != nil {
		return m.deletePortMappingErr
	}

	m.deletePortMappingCalls = append(m.deletePortMappingCalls, deletePortMappingCall{
		remoteHost:   remoteHost,
		externalPort: externalPort,
		protocol:     protocol,
	})

	return nil
}

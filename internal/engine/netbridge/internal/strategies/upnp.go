package strategies

import (
	"context"
	"errors"
	"net"

	"github.com/huin/goupnp/dcps/internetgateway2"

	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/ports"
)

// ErrNoIGD indicates no internet gateway device was found.
var ErrNoIGD = errors.New("upnp: no internet gateway device found")

// upnpIGD defines the interface for UPnP Internet Gateway Device operations.
type upnpIGD interface {
	AddPortMapping(
		remoteHost string,
		externalPort uint16,
		protocol string,
		internalPort uint16,
		internalClient string,
		enabled bool,
		description string,
		leaseDuration uint32,
	) error
	DeletePortMapping(
		remoteHost string,
		externalPort uint16,
		protocol string,
	) error
}

type upnpStrategy struct {
	discover func(ctx context.Context) ([]upnpIGD, error)
	localIP  func() (string, error)
}

// NewUPnP returns a UPnP port-forwarding strategy.
func NewUPnP() Strategy {
	return &upnpStrategy{
		discover: discoverIGDs,
		localIP:  getLocalIP,
	}
}

func (s *upnpStrategy) Name() string {
	return "UPnP"
}

func (s *upnpStrategy) Available(
	ctx context.Context,
) bool {
	igds, err := s.discover(ctx)
	if err != nil {
		return false
	}

	return len(igds) > 0
}

func (s *upnpStrategy) Forward(
	ctx context.Context,
	port int,
	protocol ports.Protocol,
) error {
	igds, err := s.discover(ctx)
	if err != nil {
		return err
	}

	if len(igds) == 0 {
		return ErrNoIGD
	}

	ip, err := s.localIP()
	if err != nil {
		return err
	}

	protos := upnpProtocols(protocol)
	igd := igds[0]

	for _, proto := range protos {
		err := igd.AddPortMapping(
			"",
			uint16(port),
			proto,
			uint16(port),
			ip,
			true,
			"quiver",
			3600,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *upnpStrategy) Reverse(
	ctx context.Context,
	port int,
	protocol ports.Protocol,
) error {
	igds, err := s.discover(ctx)
	if err != nil {
		return err
	}

	if len(igds) == 0 {
		return ErrNoIGD
	}

	protos := upnpProtocols(protocol)
	igd := igds[0]

	for _, proto := range protos {
		err := igd.DeletePortMapping("", uint16(port), proto)
		if err != nil {
			return err
		}
	}

	return nil
}

func upnpProtocols(p ports.Protocol) []string {
	switch p {
	case ports.ProtocolTCP:
		return []string{"TCP"}
	case ports.ProtocolUDP:
		return []string{"UDP"}
	case ports.ProtocolTCPUDP:
		return []string{"TCP", "UDP"}
	default:
		return []string{}
	}
}

func discoverIGDs(ctx context.Context) ([]upnpIGD, error) {
	clients, errs, err := internetgateway2.NewWANIPConnection1ClientsCtx(ctx)
	if err != nil {
		return nil, err
	}

	if len(errs) > 0 && len(clients) == 0 {
		return nil, errs[0]
	}

	result := make([]upnpIGD, len(clients))
	for i, c := range clients {
		result[i] = c
	}

	return result, nil
}

func getLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}

	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok || ipnet.IP.IsLoopback() {
			continue
		}

		if ipv4 := ipnet.IP.To4(); ipv4 != nil {
			return ipv4.String(), nil
		}
	}

	return "", errors.New("no non-loopback IPv4 address found")
}

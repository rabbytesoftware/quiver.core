package upnp

import (
	"context"
	"fmt"
	"net"

	"github.com/google/uuid"
	"github.com/huin/goupnp"
	"github.com/huin/goupnp/dcps/internetgateway2"
	"github.com/rabbytesoftware/quiver/internal/infrastructure/netbridge/operators"
	"github.com/rabbytesoftware/quiver/internal/models/networking"
	"github.com/sirupsen/logrus"
)

type UPnPOperatorImpl struct {
	client     *internetgateway2.WANIPConnection1
	device     *goupnp.RootDevice
	logger     *logrus.Logger
	discovered bool
}

func NewUPnPOperator() operators.OperatorInterface {
	return &UPnPOperatorImpl{
		discovered: false,
	}
}

func (o *UPnPOperatorImpl) discoverUPnPDevice(
	_ context.Context,
) error {
	if o.discovered {
		return nil
	}

	o.logger.Info("Discovering UPnP devices...")

	devices, errors, err := internetgateway2.NewWANIPConnection1Clients()
	if err != nil {
		o.logger.WithError(err).Warn("Failed to discover UPnP devices")
		return fmt.Errorf("UPnP discovery failed: %w", err)
	}

	if len(devices) == 0 {
		o.logger.WithField("errors", errors).Warn("No UPnP devices found")
		return fmt.Errorf("no UPnP-enabled routers found")
	}

	o.client = devices[0]
	o.device = o.client.RootDevice
	o.discovered = true

	o.logger.WithFields(logrus.Fields{
		"device": o.device.Device.DeviceType,
		"url":    o.device.URLBase,
	}).Info("UPnP device discovered successfully")

	return nil
}

func (o *UPnPOperatorImpl) IsAvailable(
	ctx context.Context,
) (bool, error) {
	err := o.discoverUPnPDevice(ctx)
	if err != nil {
		o.logger.WithError(err).Debug("UPnP not available")
		return false, nil
	}
	return true, nil
}

func (o *UPnPOperatorImpl) IsPortAvailable(
	ctx context.Context,
	port int,
	protocol networking.Protocol,
) (bool, error) {
	address := fmt.Sprintf(":%d", port)
	
	var network string
	switch protocol {
	case networking.ProtocolTCP:
		network = "tcp"
	case networking.ProtocolUDP:
		network = "udp"
	default:
		return false, fmt.Errorf("unsupported protocol: %s", protocol)
	}

	listener, err := net.Listen(network, address)
	if err != nil {
		o.logger.WithFields(logrus.Fields{
			"port":     port,
			"protocol": protocol,
		}).Debug("Port is not available locally")
		return false, nil
	}
	listener.Close()

	available, err := o.IsAvailable(ctx)
	if err != nil {
		return false, err
	}

	return available, nil
}

func (o *UPnPOperatorImpl) IsProtocolAvailable(
	ctx context.Context,
	protocol networking.Protocol,
) (bool, error) {
	if !protocol.IsValid() {
		return false, fmt.Errorf("invalid protocol: %s", protocol)
	}

	available, err := o.IsAvailable(ctx)
	if err != nil {
		return false, err
	}

	return available, nil
}

func (o *UPnPOperatorImpl) ForwardRule(
	ctx context.Context,
	rule networking.Rule,
) (networking.Port, error) {
	if !rule.IsValid() {
		return networking.Port{}, fmt.Errorf("invalid rule: protocol=%s, status=%s", 
			rule.Protocol, rule.ForwardingStatus)
	}

	port := networking.Port{
		ID:               uuid.New(),
		StartPort:        8080,
		EndPort:         	8080,
		Protocol:         rule.Protocol,
		ForwardingStatus: rule.ForwardingStatus,
	}

	return o.ForwardPort(ctx, port)
}

func (o *UPnPOperatorImpl) ForwardPort(
	ctx context.Context,
	port networking.Port,
) (networking.Port, error) {
	if err := o.discoverUPnPDevice(ctx); err != nil {
		return networking.Port{}, fmt.Errorf("UPnP not available: %w", err)
	}

	if !port.IsStartPortValid() || !port.IsEndPortValid() {
		return networking.Port{}, fmt.Errorf("invalid port range: %d-%d", 
			port.StartPort, port.EndPort)
	}

	o.logger.WithFields(logrus.Fields{
		"start_port": port.StartPort,
		"end_port":   port.EndPort,
		"protocol":   port.Protocol,
	}).Info("Forwarding port via UPnP")

	externalIP, err := o.client.GetExternalIPAddress()
	if err != nil {
		return networking.Port{}, fmt.Errorf("failed to get external IP: %w", err)
	}

	localIP, err := o.getLocalIP()
	if err != nil {
		return networking.Port{}, fmt.Errorf("failed to get local IP: %w", err)
	}

	protocol := "TCP"
	if port.Protocol.IsUDP() {
		protocol = "UDP"
	}

	description := fmt.Sprintf("Quiver-%s-%d", port.Protocol, port.StartPort)
	
	err = o.client.AddPortMapping("", uint16(port.StartPort), protocol, uint16(port.EndPort), localIP, true, description, 0)
	if err != nil {
		o.logger.WithError(err).Error("Failed to add port mapping")
		return networking.Port{}, fmt.Errorf("failed to add port mapping: %w", err)
	}

	port.ForwardingStatus = networking.ForwardingStatusEnabled

	o.logger.WithFields(logrus.Fields{
		"external_ip": externalIP,
		"local_ip":    localIP,
		"port":        port.StartPort,
		"protocol":    protocol,
	}).Info("Port forwarding successful")

	return port, nil
}

func (o *UPnPOperatorImpl) ReversePort(
	ctx context.Context,
	port networking.Port,
) (networking.Port, error) {
	if err := o.discoverUPnPDevice(ctx); err != nil {
		return networking.Port{}, fmt.Errorf("UPnP not available: %w", err)
	}

	o.logger.WithFields(logrus.Fields{
		"start_port": port.StartPort,
		"end_port":   port.EndPort,
		"protocol":   port.Protocol,
	}).Info("Reversing port forwarding via UPnP")

	protocol := "TCP"
	if port.Protocol.IsUDP() {
		protocol = "UDP"
	}

	err := o.client.DeletePortMapping("", uint16(port.StartPort), protocol)
	if err != nil {
		o.logger.WithError(err).Error("Failed to delete port mapping")
		return networking.Port{}, fmt.Errorf("failed to delete port mapping: %w", err)
	}

	port.ForwardingStatus = networking.ForwardingStatusDisabled

	o.logger.WithFields(logrus.Fields{
		"port":     port.StartPort,
		"protocol": protocol,
	}).Info("Port forwarding reversed successfully")

	return port, nil
}

func (o *UPnPOperatorImpl) GetPortForwardingStatus(
	ctx context.Context,
	port networking.Port,
) (networking.ForwardingStatus, error) {
	if err := o.discoverUPnPDevice(ctx); err != nil {
		return networking.ForwardingStatusError, fmt.Errorf("UPnP not available: %w", err)
	}

	protocol := "TCP"
	if port.Protocol.IsUDP() {
		protocol = "UDP"
	}

	internalClient, internalPort, enabled, description, leaseDuration, err := 
		o.client.GetSpecificPortMappingEntry("", uint16(port.StartPort), protocol)
	
	if err != nil {
		o.logger.WithError(err).Debug("Port mapping not found")
		return networking.ForwardingStatusDisabled, nil
	}

	o.logger.WithFields(logrus.Fields{
		"internal_ip":    internalClient,
		"internal_port":  internalPort,
		"enabled":        enabled,
		"description":    description,
		"lease_duration": leaseDuration,
	}).Debug("Port mapping status retrieved")

	if enabled {
		return networking.ForwardingStatusEnabled, nil
	}
	
	return networking.ForwardingStatusDisabled, nil
}

func (o *UPnPOperatorImpl) getLocalIP() (string, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", err
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String(), nil
}

package netbridge

import (
	"context"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/models/networking"
)

func TestNewNetbridge(t *testing.T) {
	nb := NewNetbridge()
	if nb == nil {
		t.Fatal("NewNetbridge() returned nil")
	}
}

func TestNetbridgeImpl_IsEnabled(t *testing.T) {
	nb := NewNetbridge()

	enabled := nb.IsEnabled()
	if !enabled {
		t.Error("IsEnabled() should return true")
	}
}

func TestNetbridgeImpl_IsAvailable(t *testing.T) {
	nb := NewNetbridge()

	available := nb.IsAvailable()
	if !available {
		t.Error("IsAvailable() should return true")
	}
}

func TestNetbridgeImpl_PublicIP(t *testing.T) {
	nb := NewNetbridge()
	ctx := context.Background()

	ip, err := nb.PublicIP(ctx)
	if err != nil {
		t.Errorf("PublicIP() returned error: %v", err)
	}
	if ip != "" {
		t.Error("PublicIP() should return empty string for unimplemented method")
	}
}

func TestNetbridgeImpl_LocalIP(t *testing.T) {
	nb := NewNetbridge()
	ctx := context.Background()

	ip, err := nb.LocalIP(ctx)
	if err != nil {
		t.Errorf("LocalIP() returned error: %v", err)
	}
	if ip != "" {
		t.Error("LocalIP() should return empty string for unimplemented method")
	}
}

func TestNetbridgeImpl_IsPortAvailable(t *testing.T) {
	nb := NewNetbridge()
	ctx := context.Background()

	available, err := nb.IsPortAvailable(ctx, 8080, networking.ProtocolTCP)
	if err != nil {
		t.Errorf("IsPortAvailable() returned error: %v", err)
	}
	if !available {
		t.Error("IsPortAvailable() should return true for unimplemented method")
	}
}

func TestNetbridgeImpl_ForwardPort(t *testing.T) {
	nb := NewNetbridge()
	ctx := context.Background()

	testPort := networking.Port{
		StartPort:        8080,
		EndPort:          8080,
		Protocol:         networking.ProtocolTCP,
		ForwardingStatus: networking.ForwardingStatusEnabled,
	}

	result, err := nb.ForwardPort(ctx, testPort)
	if err != nil {
		t.Errorf("ForwardPort() returned error: %v", err)
	}

	// Check that the returned port has expected values
	if result.StartPort != 8080 {
		t.Errorf("ForwardPort() returned wrong StartPort: got %d, want %d", result.StartPort, 8080)
	}
	if result.EndPort != 8080 {
		t.Errorf("ForwardPort() returned wrong EndPort: got %d, want %d", result.EndPort, 8080)
	}
	if result.Protocol != networking.ProtocolTCP {
		t.Errorf("ForwardPort() returned wrong Protocol: got %v, want %v", result.Protocol, networking.ProtocolTCP)
	}
	if result.ForwardingStatus != networking.ForwardingStatusEnabled {
		t.Errorf("ForwardPort() returned wrong ForwardingStatus: got %v, want %v", result.ForwardingStatus, networking.ForwardingStatusEnabled)
	}
}

func TestNetbridgeImpl_ReversePort(t *testing.T) {
	nb := NewNetbridge()
	ctx := context.Background()

	testPort := networking.Port{
		StartPort:        8080,
		EndPort:          8080,
		Protocol:         networking.ProtocolTCP,
		ForwardingStatus: networking.ForwardingStatusEnabled,
	}

	result, err := nb.ReversePort(ctx, testPort)
	if err != nil {
		t.Errorf("ReversePort() returned error: %v", err)
	}

	// Check that the returned port has expected values
	if result.StartPort != 8080 {
		t.Errorf("ReversePort() returned wrong StartPort: got %d, want %d", result.StartPort, 8080)
	}
	if result.EndPort != 8080 {
		t.Errorf("ReversePort() returned wrong EndPort: got %d, want %d", result.EndPort, 8080)
	}
	if result.Protocol != networking.ProtocolTCP {
		t.Errorf("ReversePort() returned wrong Protocol: got %v, want %v", result.Protocol, networking.ProtocolTCP)
	}
	if result.ForwardingStatus != networking.ForwardingStatusEnabled {
		t.Errorf("ReversePort() returned wrong ForwardingStatus: got %v, want %v", result.ForwardingStatus, networking.ForwardingStatusEnabled)
	}
}

func TestNetbridgeImpl_GetPortForwardingStatus(t *testing.T) {
	nb := NewNetbridge()
	ctx := context.Background()

	testPort := networking.Port{
		StartPort:        8080,
		EndPort:          8080,
		Protocol:         networking.ProtocolTCP,
		ForwardingStatus: networking.ForwardingStatusEnabled,
	}

	status, err := nb.GetPortForwardingStatus(ctx, testPort)
	if err != nil {
		t.Errorf("GetPortForwardingStatus() returned error: %v", err)
	}
	if status != networking.ForwardingStatusEnabled {
		t.Errorf("GetPortForwardingStatus() returned wrong status: got %v, want %v", status, networking.ForwardingStatusEnabled)
	}
}

func TestNetbridgeImpl_InterfaceCompliance(t *testing.T) {
	// Test that NetbridgeImpl implements NetbridgeInterface
	var _ NetbridgeInterface = &NetbridgeImpl{}
}

func TestNetbridgeImpl_MultipleInstances(t *testing.T) {
	nb1 := NewNetbridge()
	nb2 := NewNetbridge()

	// Both should be valid
	if nb1 == nil || nb2 == nil {
		t.Error("NewNetbridge() returned nil instance")
	}

	// Test that both instances work correctly
	if nb1.IsEnabled() != nb2.IsEnabled() {
		t.Error("Both instances should have same IsEnabled behavior")
	}

	if nb1.IsAvailable() != nb2.IsAvailable() {
		t.Error("Both instances should have same IsAvailable behavior")
	}
}

package netbridge

import (
	"context"
	"testing"

	netbridgeModels "github.com/rabbytesoftware/quiver/internal/models/netbridge"
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

	available, err := nb.IsPortAvailable(ctx, 8080)
	if err != nil {
		t.Errorf("IsPortAvailable() returned error: %v", err)
	}
	if !available {
		t.Error("IsPortAvailable() should return true for unimplemented method")
	}
}

func TestNetbridgeImpl_IsPortAvailable_DifferentPorts(t *testing.T) {
	tests := []struct {
		name    string
		portNum int
	}{
		{
			name:    "Standard port",
			portNum: 8080,
		},
		{
			name:    "Low port",
			portNum: 80,
		},
		{
			name:    "High port",
			portNum: 65535,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nb := NewNetbridge()
			ctx := context.Background()

			available, err := nb.IsPortAvailable(ctx, tt.portNum)
			if err != nil {
				t.Errorf("IsPortAvailable() returned error: %v", err)
			}
			if !available {
				t.Error("IsPortAvailable() should return true for unimplemented method")
			}
		})
	}
}

func TestNetbridgeImpl_ArePortsAvailable(t *testing.T) {
	nb := NewNetbridge()
	ctx := context.Background()

	ports := []int{8080, 8081, 8082}
	available, err := nb.ArePortsAvailable(ctx, ports)
	if err != nil {
		t.Errorf("ArePortsAvailable() returned error: %v", err)
	}
	if !available {
		t.Error("ArePortsAvailable() should return true for unimplemented method")
	}
}

func TestNetbridgeImpl_ArePortsAvailable_EmptySlice(t *testing.T) {
	nb := NewNetbridge()
	ctx := context.Background()

	ports := []int{}
	available, err := nb.ArePortsAvailable(ctx, ports)
	if err != nil {
		t.Errorf("ArePortsAvailable() returned error: %v", err)
	}
	if !available {
		t.Error("ArePortsAvailable() should return true for empty slice")
	}
}

func TestNetbridgeImpl_ForwardPort(t *testing.T) {
	nb := NewNetbridge()
	ctx := context.Background()

	rule, err := nb.ForwardPort(ctx, 8080)
	if err != nil {
		t.Errorf("ForwardPort() returned error: %v", err)
	}

	// Check that the returned rule has expected values
	if rule.StartPort != 8080 {
		t.Errorf("ForwardPort() returned wrong StartPort: got %d, want %d", rule.StartPort, 8080)
	}
	if rule.EndPort != 8080 {
		t.Errorf("ForwardPort() returned wrong EndPort: got %d, want %d", rule.EndPort, 8080)
	}
	if rule.Protocol != netbridgeModels.ProtocolTCP {
		t.Errorf("ForwardPort() returned wrong Protocol: got %v, want %v", rule.Protocol, netbridgeModels.ProtocolTCP)
	}
	if rule.ForwardingStatus != netbridgeModels.ForwardingStatusEnabled {
		t.Errorf("ForwardPort() returned wrong ForwardingStatus: got %v, want %v", rule.ForwardingStatus, netbridgeModels.ForwardingStatusEnabled)
	}
}

func TestNetbridgeImpl_ForwardPort_DifferentPorts(t *testing.T) {
	tests := []struct {
		name     string
		portNum  int
		wantPort int
	}{
		{
			name:     "Standard port",
			portNum:  8080,
			wantPort: 8080,
		},
		{
			name:     "Low port",
			portNum:  80,
			wantPort: 80,
		},
		{
			name:     "High port",
			portNum:  65535,
			wantPort: 65535,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nb := NewNetbridge()
			ctx := context.Background()

			rule, err := nb.ForwardPort(ctx, tt.portNum)
			if err != nil {
				t.Errorf("ForwardPort() returned error: %v", err)
			}

			if rule.StartPort != tt.wantPort {
				t.Errorf("ForwardPort() returned wrong StartPort: got %d, want %d", rule.StartPort, tt.wantPort)
			}
			if rule.EndPort != tt.wantPort {
				t.Errorf("ForwardPort() returned wrong EndPort: got %d, want %d", rule.EndPort, tt.wantPort)
			}
		})
	}
}

func TestNetbridgeImpl_ForwardPorts(t *testing.T) {
	nb := NewNetbridge()
	ctx := context.Background()

	ports := []int{8080, 8081, 8082}
	rules, err := nb.ForwardPorts(ctx, ports)
	if err != nil {
		t.Errorf("ForwardPorts() returned error: %v", err)
	}
	if rules == nil {
		t.Error("ForwardPorts() should return empty slice, not nil")
	}
	if len(rules) != 0 {
		t.Error("ForwardPorts() should return empty slice for unimplemented method")
	}
}

func TestNetbridgeImpl_ForwardPorts_EmptySlice(t *testing.T) {
	nb := NewNetbridge()
	ctx := context.Background()

	ports := []int{}
	rules, err := nb.ForwardPorts(ctx, ports)
	if err != nil {
		t.Errorf("ForwardPorts() returned error: %v", err)
	}
	if rules == nil {
		t.Error("ForwardPorts() should return empty slice, not nil")
	}
	if len(rules) != 0 {
		t.Error("ForwardPorts() should return empty slice for empty input")
	}
}

func TestNetbridgeImpl_ReversePort(t *testing.T) {
	nb := NewNetbridge()
	ctx := context.Background()

	rule, err := nb.ReversePort(ctx, 8080)
	if err != nil {
		t.Errorf("ReversePort() returned error: %v", err)
	}

	// Check that the returned rule has expected values
	if rule.StartPort != 8080 {
		t.Errorf("ReversePort() returned wrong StartPort: got %d, want %d", rule.StartPort, 8080)
	}
	if rule.EndPort != 8080 {
		t.Errorf("ReversePort() returned wrong EndPort: got %d, want %d", rule.EndPort, 8080)
	}
	if rule.Protocol != netbridgeModels.ProtocolTCP {
		t.Errorf("ReversePort() returned wrong Protocol: got %v, want %v", rule.Protocol, netbridgeModels.ProtocolTCP)
	}
	if rule.ForwardingStatus != netbridgeModels.ForwardingStatusDisabled {
		t.Errorf("ReversePort() returned wrong ForwardingStatus: got %v, want %v", rule.ForwardingStatus, netbridgeModels.ForwardingStatusDisabled)
	}
}

func TestNetbridgeImpl_ReversePorts(t *testing.T) {
	nb := NewNetbridge()
	ctx := context.Background()

	ports := []int{8080, 8081, 8082}
	rules, err := nb.ReversePorts(ctx, ports)
	if err != nil {
		t.Errorf("ReversePorts() returned error: %v", err)
	}
	if rules == nil {
		t.Error("ReversePorts() should return empty slice, not nil")
	}
	if len(rules) != 0 {
		t.Error("ReversePorts() should return empty slice for unimplemented method")
	}
}

func TestNetbridgeImpl_ReversePorts_EmptySlice(t *testing.T) {
	nb := NewNetbridge()
	ctx := context.Background()

	ports := []int{}
	rules, err := nb.ReversePorts(ctx, ports)
	if err != nil {
		t.Errorf("ReversePorts() returned error: %v", err)
	}
	if rules == nil {
		t.Error("ReversePorts() should return empty slice, not nil")
	}
	if len(rules) != 0 {
		t.Error("ReversePorts() should return empty slice for empty input")
	}
}

func TestNetbridgeImpl_GetPortForwardingStatus(t *testing.T) {
	nb := NewNetbridge()
	ctx := context.Background()

	status, err := nb.GetPortForwardingStatus(ctx, 8080)
	if err != nil {
		t.Errorf("GetPortForwardingStatus() returned error: %v", err)
	}
	if status != netbridgeModels.ForwardingStatusEnabled {
		t.Errorf("GetPortForwardingStatus() returned wrong status: got %v, want %v", status, netbridgeModels.ForwardingStatusEnabled)
	}
}

func TestNetbridgeImpl_GetPortForwardingStatuses(t *testing.T) {
	nb := NewNetbridge()
	ctx := context.Background()

	ports := []int{8080, 8081, 8082}
	statuses, err := nb.GetPortForwardingStatuses(ctx, ports)
	if err != nil {
		t.Errorf("GetPortForwardingStatuses() returned error: %v", err)
	}
	if statuses == nil {
		t.Error("GetPortForwardingStatuses() should return empty slice, not nil")
	}
	if len(statuses) != 0 {
		t.Error("GetPortForwardingStatuses() should return empty slice for unimplemented method")
	}
}

func TestNetbridgeImpl_GetPortForwardingStatuses_EmptySlice(t *testing.T) {
	nb := NewNetbridge()
	ctx := context.Background()

	ports := []int{}
	statuses, err := nb.GetPortForwardingStatuses(ctx, ports)
	if err != nil {
		t.Errorf("GetPortForwardingStatuses() returned error: %v", err)
	}
	if statuses == nil {
		t.Error("GetPortForwardingStatuses() should return empty slice, not nil")
	}
	if len(statuses) != 0 {
		t.Error("GetPortForwardingStatuses() should return empty slice for empty input")
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

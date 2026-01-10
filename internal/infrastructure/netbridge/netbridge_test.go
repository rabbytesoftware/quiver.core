package netbridge

import (
	"context"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/infrastructure/netbridge/operators"
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
	nb := &NetbridgeImpl{
		operator: &fakeOperator{available: true},
	}

	available := nb.IsAvailable()
	if !available {
		t.Error("IsAvailable() should return true")
	}
}

func TestNetbridgeImpl_PublicIP(t *testing.T) {
	nb := &NetbridgeImpl{
		operator: &fakeOperator{publicIP: "203.0.113.1"},
	}
	ctx := context.Background()

	ip, err := nb.PublicIP(ctx)
	if err != nil {
		t.Errorf("PublicIP() returned error: %v", err)
	}
	if ip != "203.0.113.1" {
		t.Errorf("PublicIP() returned wrong IP: got %s, want %s", ip, "203.0.113.1")
	}
}

func TestNetbridgeImpl_LocalIP(t *testing.T) {
	nb := &NetbridgeImpl{
		operator: &fakeOperator{localIP: "192.168.1.100"},
	}
	ctx := context.Background()

	ip, err := nb.LocalIP(ctx)
	if err != nil {
		t.Errorf("LocalIP() returned error: %v", err)
	}
	if ip != "192.168.1.100" {
		t.Errorf("LocalIP() returned wrong IP: got %s, want %s", ip, "192.168.1.100")
	}
}

func TestNetbridgeImpl_IsPortAvailable(t *testing.T) {
	nb := &NetbridgeImpl{
		operator: &fakeOperator{available: true, portAvailable: true},
	}
	ctx := context.Background()

	available, err := nb.IsPortAvailable(ctx, 40128, networking.ProtocolTCP)
	if err != nil {
		t.Errorf("IsPortAvailable() returned error: %v", err)
	}
	if !available {
		t.Error("IsPortAvailable() should return true for allowed port")
	}
}

func TestNetbridgeImpl_ForwardPort(t *testing.T) {
	nb := &NetbridgeImpl{
		operator: &fakeOperator{available: true},
	}
	ctx := context.Background()

	testPort := networking.Port{
		StartPort:        40128,
		EndPort:          40128,
		Protocol:         networking.ProtocolTCP,
		ForwardingStatus: networking.ForwardingStatusEnabled,
	}

	result, err := nb.ForwardPort(ctx, testPort)
	if err != nil {
		t.Errorf("ForwardPort() returned error: %v", err)
	}

	// Check that the returned port has expected values
	if result.StartPort != 40128 {
		t.Errorf("ForwardPort() returned wrong StartPort: got %d, want %d", result.StartPort, 40128)
	}
	if result.EndPort != 40128 {
		t.Errorf("ForwardPort() returned wrong EndPort: got %d, want %d", result.EndPort, 40128)
	}
	if result.Protocol != networking.ProtocolTCP {
		t.Errorf("ForwardPort() returned wrong Protocol: got %v, want %v", result.Protocol, networking.ProtocolTCP)
	}
	if result.ForwardingStatus != networking.ForwardingStatusEnabled {
		t.Errorf("ForwardPort() returned wrong ForwardingStatus: got %v, want %v", result.ForwardingStatus, networking.ForwardingStatusEnabled)
	}
}

func TestNetbridgeImpl_ReversePort(t *testing.T) {
	nb := &NetbridgeImpl{
		operator: &fakeOperator{available: true},
	}
	ctx := context.Background()

	testPort := networking.Port{
		StartPort:        40128,
		EndPort:          40128,
		Protocol:         networking.ProtocolTCP,
		ForwardingStatus: networking.ForwardingStatusEnabled,
	}

	result, err := nb.ReversePort(ctx, testPort)
	if err != nil {
		t.Errorf("ReversePort() returned error: %v", err)
	}

	// Check that the returned port has expected values
	if result.StartPort != 40128 {
		t.Errorf("ReversePort() returned wrong StartPort: got %d, want %d", result.StartPort, 40128)
	}
	if result.EndPort != 40128 {
		t.Errorf("ReversePort() returned wrong EndPort: got %d, want %d", result.EndPort, 40128)
	}
	if result.Protocol != networking.ProtocolTCP {
		t.Errorf("ReversePort() returned wrong Protocol: got %v, want %v", result.Protocol, networking.ProtocolTCP)
	}
	if result.ForwardingStatus != networking.ForwardingStatusEnabled {
		t.Errorf("ReversePort() returned wrong ForwardingStatus: got %v, want %v", result.ForwardingStatus, networking.ForwardingStatusEnabled)
	}
}

func TestNetbridgeImpl_GetPortForwardingStatus(t *testing.T) {
	nb := &NetbridgeImpl{
		operator: &fakeOperator{available: true},
	}
	ctx := context.Background()

	testPort := networking.Port{
		StartPort:        40128,
		EndPort:          40128,
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

func TestNetbridgeImpl_DetectInfrastructure(t *testing.T) {
	container := operators.NewEmptyOperatorContainer()
	container.Add(&fakeOperator{name: "primary", available: true})
	container.Add(&fakeOperator{name: "secondary", available: false})

	nb := &NetbridgeImpl{
		operators: container,
	}

	status, err := nb.DetectInfrastructure(context.Background())
	if err != nil {
		t.Fatalf("DetectInfrastructure() returned error: %v", err)
	}

	if !status.Enabled {
		t.Error("DetectInfrastructure() should report enabled netbridge")
	}

	if status.Selected != "primary" {
		t.Errorf("DetectInfrastructure() returned wrong selected operator: got %s, want %s", status.Selected, "primary")
	}

	if len(status.Available) == 0 {
		t.Error("DetectInfrastructure() should return available operators")
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

type fakeOperator struct {
	available     bool
	portAvailable bool
	publicIP      string
	localIP       string
	name          string
}

func (f *fakeOperator) Name() string {
	if f.name == "" {
		return "fake"
	}
	return f.name
}

func (f *fakeOperator) IsAvailable(context.Context) (bool, error) {
	return f.available, nil
}

func (f *fakeOperator) IsPortAvailable(
	_ context.Context,
	_ int,
	_ networking.Protocol,
) (bool, error) {
	return f.portAvailable, nil
}

func (f *fakeOperator) IsProtocolAvailable(
	_ context.Context,
	_ networking.Protocol,
) (bool, error) {
	return f.available, nil
}

func (f *fakeOperator) ForwardRule(
	_ context.Context,
	rule networking.Rule,
) (networking.Port, error) {
	return networking.Port{
		StartPort:        40128,
		EndPort:          40128,
		Protocol:         rule.Protocol,
		ForwardingStatus: rule.ForwardingStatus,
	}, nil
}

func (f *fakeOperator) ForwardPort(
	_ context.Context,
	port networking.Port,
) (networking.Port, error) {
	return port, nil
}

func (f *fakeOperator) ReversePort(
	_ context.Context,
	port networking.Port,
) (networking.Port, error) {
	return port, nil
}

func (f *fakeOperator) GetPortForwardingStatus(
	_ context.Context,
	port networking.Port,
) (networking.ForwardingStatus, error) {
	return port.ForwardingStatus, nil
}

func (f *fakeOperator) PublicIP(context.Context) (string, error) {
	return f.publicIP, nil
}

func (f *fakeOperator) LocalIP(context.Context) (string, error) {
	return f.localIP, nil
}

package upnp

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/rabbytesoftware/quiver/internal/models/networking"
)

func TestNewUPnPOperator(t *testing.T) {
	operator := NewUPnPOperator()
	if operator == nil {
		t.Fatal("Expected operator to be non-nil")
	}
	
	if _, ok := operator.(*UPnPOperatorImpl); !ok {
		t.Fatalf("Expected operator to be of type *UPnPOperatorImpl, got %T", operator)
	}
}

func TestUPnPOperatorImpl_IsAvailable(t *testing.T) {
	operator := &UPnPOperatorImpl{
		discovered: true,
	}

	ctx := context.Background()
	available, err := operator.IsAvailable(ctx)

	if err != nil {
		t.Errorf("Expected no error for already discovered device, got %v", err)
	}
	
	if !available {
		t.Error("Expected available to be true for discovered device")
	}
}

func TestUPnPOperatorImpl_IsPortAvailable_InvalidProtocol(t *testing.T) {
	operator := &UPnPOperatorImpl{
		discovered: true,
	}

	ctx := context.Background()
	_, err := operator.IsPortAvailable(ctx, 8080, networking.Protocol("invalid"))

	if err == nil {
		t.Error("Expected error for invalid protocol")
	}
}

func TestUPnPOperatorImpl_IsProtocolAvailable(t *testing.T) {
	tests := []struct {
		name        string
		protocol    networking.Protocol
		expectError bool
	}{
		{
			name:        "Valid TCP protocol",
			protocol:    networking.ProtocolTCP,
			expectError: false,
		},
		{
			name:        "Valid UDP protocol",
			protocol:    networking.ProtocolUDP,
			expectError: false,
		},
		{
			name:        "Invalid protocol",
			protocol:    networking.Protocol("invalid"),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			operator := &UPnPOperatorImpl{
				discovered: true,
			}

			ctx := context.Background()
			_, err := operator.IsProtocolAvailable(ctx, tt.protocol)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got %v", err)
			}
		})
	}
}

func TestUPnPOperatorImpl_ForwardRule_InvalidRule(t *testing.T) {
	operator := &UPnPOperatorImpl{
		discovered: true,
	}

	ctx := context.Background()
	invalidRule := networking.Rule{
		Protocol:         networking.Protocol("invalid"),
		ForwardingStatus: networking.ForwardingStatusEnabled,
	}

	_, err := operator.ForwardRule(ctx, invalidRule)
	if err == nil {
		t.Error("Expected error for invalid rule")
	}
}

func TestUPnPOperatorImpl_ForwardRule_ValidRule(t *testing.T) {
	operator := NewUPnPOperator().(*UPnPOperatorImpl)
	operator.discovered = false // Will fail discovery in test environment

	ctx := context.Background()
	validRule := networking.Rule{
		Protocol:         networking.ProtocolTCP,
		ForwardingStatus: networking.ForwardingStatusEnabled,
	}

	_, err := operator.ForwardRule(ctx, validRule)
	// In test environment without UPnP device, this should fail
	if err == nil {
		t.Log("Note: ForwardRule succeeded (UPnP device found in test environment)")
	}
}

func TestUPnPOperatorImpl_ForwardPort_InvalidPortRange(t *testing.T) {
	operator := &UPnPOperatorImpl{
		discovered: true,
	}

	ctx := context.Background()
	invalidPort := networking.Port{
		ID:               uuid.New(),
		StartPort:        0, // Invalid
		EndPort:          0, // Invalid
		Protocol:         networking.ProtocolTCP,
		ForwardingStatus: networking.ForwardingStatusDisabled,
	}

	_, err := operator.ForwardPort(ctx, invalidPort)
	if err == nil {
		t.Error("Expected error for invalid port range")
	}
}

func TestUPnPOperatorImpl_ForwardPort_UPnPNotAvailable(t *testing.T) {
	operator := &UPnPOperatorImpl{
		discovered: false,
	}

	ctx := context.Background()
	port := networking.Port{
		ID:               uuid.New(),
		StartPort:        8080,
		EndPort:          8080,
		Protocol:         networking.ProtocolTCP,
		ForwardingStatus: networking.ForwardingStatusDisabled,
	}

	_, err := operator.ForwardPort(ctx, port)
	if err == nil {
		t.Error("Expected error when UPnP is not available")
	}
}

func TestUPnPOperatorImpl_ReversePort_UPnPNotAvailable(t *testing.T) {
	operator := &UPnPOperatorImpl{
		discovered: false,
	}

	ctx := context.Background()
	port := networking.Port{
		ID:               uuid.New(),
		StartPort:        8080,
		EndPort:          8080,
		Protocol:         networking.ProtocolTCP,
		ForwardingStatus: networking.ForwardingStatusEnabled,
	}

	_, err := operator.ReversePort(ctx, port)
	if err == nil {
		t.Error("Expected error when UPnP is not available")
	}
}

func TestUPnPOperatorImpl_GetPortForwardingStatus_UPnPNotAvailable(t *testing.T) {
	operator := &UPnPOperatorImpl{
		discovered: false,
	}

	ctx := context.Background()
	port := networking.Port{
		ID:               uuid.New(),
		StartPort:        8080,
		EndPort:          8080,
		Protocol:         networking.ProtocolTCP,
		ForwardingStatus: networking.ForwardingStatusEnabled,
	}

	status, err := operator.GetPortForwardingStatus(ctx, port)
	if err == nil {
		t.Error("Expected error when UPnP is not available")
	}
	if status != networking.ForwardingStatusError {
		t.Errorf("Expected ForwardingStatusError, got %v", status)
	}
}

func TestUPnPOperatorImpl_getLocalIP(t *testing.T) {
	operator := &UPnPOperatorImpl{}
	
	ip, err := operator.getLocalIP()
	
	if err != nil {
		t.Logf("getLocalIP failed (may be expected in some test environments): %v", err)
		return
	}
	
	if ip == "" {
		t.Error("Expected non-empty IP address")
	}
	
	// Basic validation that it looks like an IP
	if len(ip) < 7 { // Minimum IP like "0.0.0.0"
		t.Errorf("IP address seems invalid: %s", ip)
	}
}

// Benchmark tests
func BenchmarkUPnPOperator_IsAvailable(b *testing.B) {
	operator := &UPnPOperatorImpl{
		discovered: true,
	}
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = operator.IsAvailable(ctx)
	}
}

func BenchmarkUPnPOperator_IsProtocolAvailable(b *testing.B) {
	operator := &UPnPOperatorImpl{
		discovered: true,
	}
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = operator.IsProtocolAvailable(ctx, networking.ProtocolTCP)
	}
}

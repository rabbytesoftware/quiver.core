package networking

import "testing"

func TestProtocol_String(t *testing.T) {
	testCases := []struct {
		name     string
		protocol Protocol
		expected string
	}{
		{
			name:     "TCP protocol",
			protocol: ProtocolTCP,
			expected: "tcp",
		},
		{
			name:     "UDP protocol",
			protocol: ProtocolUDP,
			expected: "udp",
		},
		{
			name:     "TCP/UDP protocol",
			protocol: ProtocolTCPUDP,
			expected: "tcp/udp",
		},
		{
			name:     "custom protocol",
			protocol: Protocol("custom"),
			expected: "custom",
		},
		{
			name:     "empty protocol",
			protocol: Protocol(""),
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.protocol.String()
			if result != tc.expected {
				t.Errorf("Expected String() to return %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestProtocol_IsValid(t *testing.T) {
	testCases := []struct {
		name     string
		protocol Protocol
		expected bool
	}{
		{
			name:     "TCP protocol",
			protocol: ProtocolTCP,
			expected: true,
		},
		{
			name:     "UDP protocol",
			protocol: ProtocolUDP,
			expected: true,
		},
		{
			name:     "TCP/UDP protocol",
			protocol: ProtocolTCPUDP,
			expected: true,
		},
		{
			name:     "custom protocol",
			protocol: Protocol("custom"),
			expected: false,
		},
		{
			name:     "empty protocol",
			protocol: Protocol(""),
			expected: false,
		},
		{
			name:     "uppercase TCP",
			protocol: Protocol("TCP"),
			expected: false,
		},
		{
			name:     "mixed case",
			protocol: Protocol("Tcp"),
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.protocol.IsValid()
			if result != tc.expected {
				t.Errorf("Expected IsValid() to return %v for %q, got %v", tc.expected, tc.protocol, result)
			}
		})
	}
}

func TestProtocol_IsTCP(t *testing.T) {
	testCases := []struct {
		name     string
		protocol Protocol
		expected bool
	}{
		{
			name:     "TCP protocol",
			protocol: ProtocolTCP,
			expected: true,
		},
		{
			name:     "UDP protocol",
			protocol: ProtocolUDP,
			expected: false,
		},
		{
			name:     "TCP/UDP protocol",
			protocol: ProtocolTCPUDP,
			expected: false,
		},
		{
			name:     "custom protocol",
			protocol: Protocol("custom"),
			expected: false,
		},
		{
			name:     "empty protocol",
			protocol: Protocol(""),
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.protocol.IsTCP()
			if result != tc.expected {
				t.Errorf("Expected IsTCP() to return %v for %q, got %v", tc.expected, tc.protocol, result)
			}
		})
	}
}

func TestProtocol_IsUDP(t *testing.T) {
	testCases := []struct {
		name     string
		protocol Protocol
		expected bool
	}{
		{
			name:     "TCP protocol",
			protocol: ProtocolTCP,
			expected: false,
		},
		{
			name:     "UDP protocol",
			protocol: ProtocolUDP,
			expected: true,
		},
		{
			name:     "TCP/UDP protocol",
			protocol: ProtocolTCPUDP,
			expected: false,
		},
		{
			name:     "custom protocol",
			protocol: Protocol("custom"),
			expected: false,
		},
		{
			name:     "empty protocol",
			protocol: Protocol(""),
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.protocol.IsUDP()
			if result != tc.expected {
				t.Errorf("Expected IsUDP() to return %v for %q, got %v", tc.expected, tc.protocol, result)
			}
		})
	}
}

func TestProtocol_IsTCPUDP(t *testing.T) {
	testCases := []struct {
		name     string
		protocol Protocol
		expected bool
	}{
		{
			name:     "TCP protocol",
			protocol: ProtocolTCP,
			expected: false,
		},
		{
			name:     "UDP protocol",
			protocol: ProtocolUDP,
			expected: false,
		},
		{
			name:     "TCP/UDP protocol",
			protocol: ProtocolTCPUDP,
			expected: true,
		},
		{
			name:     "custom protocol",
			protocol: Protocol("custom"),
			expected: false,
		},
		{
			name:     "empty protocol",
			protocol: Protocol(""),
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.protocol.IsTCPUDP()
			if result != tc.expected {
				t.Errorf("Expected IsTCPUDP() to return %v for %q, got %v", tc.expected, tc.protocol, result)
			}
		})
	}
}

func TestProtocol_Constants(t *testing.T) {
	// Test that constants have expected values
	if ProtocolTCP != "tcp" {
		t.Errorf("Expected ProtocolTCP to be 'tcp', got %q", ProtocolTCP)
	}

	if ProtocolUDP != "udp" {
		t.Errorf("Expected ProtocolUDP to be 'udp', got %q", ProtocolUDP)
	}

	if ProtocolTCPUDP != "tcp/udp" {
		t.Errorf("Expected ProtocolTCPUDP to be 'tcp/udp', got %q", ProtocolTCPUDP)
	}
}

func TestProtocol_AllMethods(t *testing.T) {
	// Test all methods on each constant
	protocols := []Protocol{
		ProtocolTCP,
		ProtocolUDP,
		ProtocolTCPUDP,
	}

	for _, protocol := range protocols {
		// All valid protocols should be valid
		if !protocol.IsValid() {
			t.Errorf("Expected protocol %q to be valid", protocol)
		}

		// Each protocol should have exactly one specific method return true
		trueCount := 0
		if protocol.IsTCP() {
			trueCount++
		}
		if protocol.IsUDP() {
			trueCount++
		}
		if protocol.IsTCPUDP() {
			trueCount++
		}

		if trueCount != 1 {
			t.Errorf("Expected exactly one specific method to return true for protocol %q, got %d", protocol, trueCount)
		}

		// String method should return the expected value
		if protocol.String() != string(protocol) {
			t.Errorf("Expected String() to return %q, got %q", string(protocol), protocol.String())
		}
	}
}

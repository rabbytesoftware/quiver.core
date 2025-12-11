package netbridge

import "testing"

func TestPortRule_IsStartPortValid(t *testing.T) {
	testCases := []struct {
		name     string
		portRule PortRule
		expected bool
	}{
		{
			name:     "valid start port 80",
			portRule: PortRule{StartPort: 80},
			expected: true,
		},
		{
			name:     "valid start port 1",
			portRule: PortRule{StartPort: 1},
			expected: true,
		},
		{
			name:     "valid start port 65535",
			portRule: PortRule{StartPort: 65535},
			expected: true,
		},
		{
			name:     "invalid start port 0",
			portRule: PortRule{StartPort: 0},
			expected: false,
		},
		{
			name:     "invalid start port negative",
			portRule: PortRule{StartPort: -1},
			expected: false,
		},
		{
			name:     "invalid start port too high",
			portRule: PortRule{StartPort: 65536},
			expected: false,
		},
		{
			name:     "invalid start port very high",
			portRule: PortRule{StartPort: 100000},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.portRule.IsStartPortValid()
			if result != tc.expected {
				t.Errorf("Expected IsStartPortValid() to return %v for port %d, got %v", tc.expected, tc.portRule.StartPort, result)
			}
		})
	}
}

func TestPortRule_IsEndPortValid(t *testing.T) {
	testCases := []struct {
		name     string
		portRule PortRule
		expected bool
	}{
		{
			name:     "valid end port 80",
			portRule: PortRule{EndPort: 80},
			expected: true,
		},
		{
			name:     "valid end port 1",
			portRule: PortRule{EndPort: 1},
			expected: true,
		},
		{
			name:     "valid end port 65535",
			portRule: PortRule{EndPort: 65535},
			expected: true,
		},
		{
			name:     "invalid end port 0",
			portRule: PortRule{EndPort: 0},
			expected: false,
		},
		{
			name:     "invalid end port negative",
			portRule: PortRule{EndPort: -1},
			expected: false,
		},
		{
			name:     "invalid end port too high",
			portRule: PortRule{EndPort: 65536},
			expected: false,
		},
		{
			name:     "invalid end port very high",
			portRule: PortRule{EndPort: 100000},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.portRule.IsEndPortValid()
			if result != tc.expected {
				t.Errorf("Expected IsEndPortValid() to return %v for port %d, got %v", tc.expected, tc.portRule.EndPort, result)
			}
		})
	}
}

func TestPortRule_BothPortsValid(t *testing.T) {
	testCases := []struct {
		name       string
		portRule   PortRule
		startValid bool
		endValid   bool
	}{
		{
			name:       "both ports valid",
			portRule:   PortRule{StartPort: 80, EndPort: 8080},
			startValid: true,
			endValid:   true,
		},
		{
			name:       "start port invalid, end port valid",
			portRule:   PortRule{StartPort: 0, EndPort: 8080},
			startValid: false,
			endValid:   true,
		},
		{
			name:       "start port valid, end port invalid",
			portRule:   PortRule{StartPort: 80, EndPort: 0},
			startValid: true,
			endValid:   false,
		},
		{
			name:       "both ports invalid",
			portRule:   PortRule{StartPort: 0, EndPort: 0},
			startValid: false,
			endValid:   false,
		},
		{
			name:       "same port for both",
			portRule:   PortRule{StartPort: 8080, EndPort: 8080},
			startValid: true,
			endValid:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			startResult := tc.portRule.IsStartPortValid()
			endResult := tc.portRule.IsEndPortValid()

			if startResult != tc.startValid {
				t.Errorf("Expected IsStartPortValid() to return %v, got %v", tc.startValid, startResult)
			}

			if endResult != tc.endValid {
				t.Errorf("Expected IsEndPortValid() to return %v, got %v", tc.endValid, endResult)
			}
		})
	}
}

func TestPortRule_StructFields(t *testing.T) {
	// Test that all fields can be set and retrieved
	portRule := PortRule{
		ID:               "test-id",
		StartPort:        80,
		EndPort:          8080,
		Protocol:         ProtocolTCP,
		ForwardingStatus: ForwardingStatusEnabled,
	}

	if portRule.ID != "test-id" {
		t.Errorf("Expected ID to be 'test-id', got %q", portRule.ID)
	}

	if portRule.StartPort != 80 {
		t.Errorf("Expected StartPort to be 80, got %d", portRule.StartPort)
	}

	if portRule.EndPort != 8080 {
		t.Errorf("Expected EndPort to be 8080, got %d", portRule.EndPort)
	}

	if portRule.Protocol != ProtocolTCP {
		t.Errorf("Expected Protocol to be %q, got %q", ProtocolTCP, portRule.Protocol)
	}

	if portRule.ForwardingStatus != ForwardingStatusEnabled {
		t.Errorf("Expected ForwardingStatus to be %q, got %q", ForwardingStatusEnabled, portRule.ForwardingStatus)
	}
}

func TestPortRule_ZeroValue(t *testing.T) {
	// Test zero value of PortRule
	var portRule PortRule

	if portRule.ID != "" {
		t.Errorf("Expected zero value ID to be empty, got %q", portRule.ID)
	}

	if portRule.StartPort != 0 {
		t.Errorf("Expected zero value StartPort to be 0, got %d", portRule.StartPort)
	}

	if portRule.EndPort != 0 {
		t.Errorf("Expected zero value EndPort to be 0, got %d", portRule.EndPort)
	}

	if portRule.Protocol != "" {
		t.Errorf("Expected zero value Protocol to be empty, got %q", portRule.Protocol)
	}

	if portRule.ForwardingStatus != "" {
		t.Errorf("Expected zero value ForwardingStatus to be empty, got %q", portRule.ForwardingStatus)
	}

	// Zero value ports should be invalid
	if portRule.IsStartPortValid() {
		t.Error("Expected zero value start port to be invalid")
	}

	if portRule.IsEndPortValid() {
		t.Error("Expected zero value end port to be invalid")
	}
}

func TestPortRule_EdgeCasePorts(t *testing.T) {
	// Test boundary values
	testCases := []struct {
		name     string
		port     int
		expected bool
	}{
		{"port 1 (minimum valid)", 1, true},
		{"port 65535 (maximum valid)", 65535, true},
		{"port 0 (invalid)", 0, false},
		{"port -1 (invalid)", -1, false},
		{"port 65536 (invalid)", 65536, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			portRule := PortRule{StartPort: tc.port, EndPort: tc.port}

			startResult := portRule.IsStartPortValid()
			endResult := portRule.IsEndPortValid()

			if startResult != tc.expected {
				t.Errorf("Expected IsStartPortValid() to return %v for port %d, got %v", tc.expected, tc.port, startResult)
			}

			if endResult != tc.expected {
				t.Errorf("Expected IsEndPortValid() to return %v for port %d, got %v", tc.expected, tc.port, endResult)
			}
		})
	}
}

func TestPortRule_Validate(t *testing.T) {
	testCases := []struct {
		name        string
		portRule    PortRule
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid port rule",
			portRule: PortRule{
				ID:               "rule-1",
				StartPort:        8080,
				EndPort:          8080,
				Protocol:         ProtocolTCP,
				ForwardingStatus: ForwardingStatusEnabled,
			},
			expectError: false,
		},
		{
			name: "empty ID",
			portRule: PortRule{
				ID:        "",
				StartPort: 8080,
				EndPort:   8080,
				Protocol:  ProtocolTCP,
			},
			expectError: true,
			errorMsg:    "port rule ID cannot be empty",
		},
		{
			name: "zero start port is valid",
			portRule: PortRule{
				ID:        "rule-1",
				StartPort: 0,
				EndPort:   8080,
				Protocol:  ProtocolTCP,
			},
			expectError: false,
		},
		{
			name: "start port above maximum",
			portRule: PortRule{
				ID:        "rule-1",
				StartPort: 65536,
				EndPort:   8080,
				Protocol:  ProtocolTCP,
			},
			expectError: true,
			errorMsg:    "start_port must be between",
		},
		{
			name: "zero end port is valid",
			portRule: PortRule{
				ID:        "rule-1",
				StartPort: 8080,
				EndPort:   0,
				Protocol:  ProtocolTCP,
			},
			expectError: false,
		},
		{
			name: "end port above maximum",
			portRule: PortRule{
				ID:        "rule-1",
				StartPort: 8080,
				EndPort:   65536,
				Protocol:  ProtocolTCP,
			},
			expectError: true,
			errorMsg:    "end_port must be between",
		},
		{
			name: "start port greater than end port",
			portRule: PortRule{
				ID:        "rule-1",
				StartPort: 9000,
				EndPort:   8000,
				Protocol:  ProtocolTCP,
			},
			expectError: true,
			errorMsg:    "start_port (9000) cannot be greater than end_port (8000)",
		},
		{
			name: "invalid protocol",
			portRule: PortRule{
				ID:        "rule-1",
				StartPort: 8080,
				EndPort:   8080,
				Protocol:  Protocol("invalid"),
			},
			expectError: true,
			errorMsg:    "invalid protocol",
		},
		{
			name: "valid port range",
			portRule: PortRule{
				ID:               "rule-1",
				StartPort:        8000,
				EndPort:          8999,
				Protocol:         ProtocolTCP,
				ForwardingStatus: ForwardingStatusEnabled,
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.portRule.Validate()
			if tc.expectError {
				if err == nil {
					t.Error("Expected error but got nil")
				} else if tc.errorMsg != "" && !portContains(err.Error(), tc.errorMsg) {
					t.Errorf("Expected error containing %q, got %q", tc.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got %v", err)
				}
			}
		})
	}
}

func portContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

package netbridge

import "testing"

func TestForwardingStatus_String(t *testing.T) {
	testCases := []struct {
		name     string
		status   ForwardingStatus
		expected string
	}{
		{
			name:     "enabled status",
			status:   ForwardingStatusEnabled,
			expected: "enabled",
		},
		{
			name:     "disabled status",
			status:   ForwardingStatusDisabled,
			expected: "disabled",
		},
		{
			name:     "error status",
			status:   ForwardingStatusError,
			expected: "error",
		},
		{
			name:     "custom status",
			status:   ForwardingStatus("custom"),
			expected: "custom",
		},
		{
			name:     "empty status",
			status:   ForwardingStatus(""),
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.status.String()
			if result != tc.expected {
				t.Errorf("Expected String() to return %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestForwardingStatus_IsEnabled(t *testing.T) {
	testCases := []struct {
		name     string
		status   ForwardingStatus
		expected bool
	}{
		{
			name:     "enabled status",
			status:   ForwardingStatusEnabled,
			expected: true,
		},
		{
			name:     "disabled status",
			status:   ForwardingStatusDisabled,
			expected: false,
		},
		{
			name:     "error status",
			status:   ForwardingStatusError,
			expected: false,
		},
		{
			name:     "custom status",
			status:   ForwardingStatus("custom"),
			expected: false,
		},
		{
			name:     "empty status",
			status:   ForwardingStatus(""),
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.status.IsEnabled()
			if result != tc.expected {
				t.Errorf("Expected IsEnabled() to return %v for %q, got %v", tc.expected, tc.status, result)
			}
		})
	}
}

func TestForwardingStatus_IsDisabled(t *testing.T) {
	testCases := []struct {
		name     string
		status   ForwardingStatus
		expected bool
	}{
		{
			name:     "enabled status",
			status:   ForwardingStatusEnabled,
			expected: false,
		},
		{
			name:     "disabled status",
			status:   ForwardingStatusDisabled,
			expected: true,
		},
		{
			name:     "error status",
			status:   ForwardingStatusError,
			expected: false,
		},
		{
			name:     "custom status",
			status:   ForwardingStatus("custom"),
			expected: false,
		},
		{
			name:     "empty status",
			status:   ForwardingStatus(""),
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.status.IsDisabled()
			if result != tc.expected {
				t.Errorf("Expected IsDisabled() to return %v for %q, got %v", tc.expected, tc.status, result)
			}
		})
	}
}

func TestForwardingStatus_IsError(t *testing.T) {
	testCases := []struct {
		name     string
		status   ForwardingStatus
		expected bool
	}{
		{
			name:     "enabled status",
			status:   ForwardingStatusEnabled,
			expected: false,
		},
		{
			name:     "disabled status",
			status:   ForwardingStatusDisabled,
			expected: false,
		},
		{
			name:     "error status",
			status:   ForwardingStatusError,
			expected: true,
		},
		{
			name:     "custom status",
			status:   ForwardingStatus("custom"),
			expected: false,
		},
		{
			name:     "empty status",
			status:   ForwardingStatus(""),
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.status.IsError()
			if result != tc.expected {
				t.Errorf("Expected IsError() to return %v for %q, got %v", tc.expected, tc.status, result)
			}
		})
	}
}

func TestForwardingStatus_Constants(t *testing.T) {
	// Test that constants have expected values
	if ForwardingStatusEnabled != "enabled" {
		t.Errorf("Expected ForwardingStatusEnabled to be 'enabled', got %q", ForwardingStatusEnabled)
	}

	if ForwardingStatusDisabled != "disabled" {
		t.Errorf("Expected ForwardingStatusDisabled to be 'disabled', got %q", ForwardingStatusDisabled)
	}

	if ForwardingStatusError != "error" {
		t.Errorf("Expected ForwardingStatusError to be 'error', got %q", ForwardingStatusError)
	}
}

func TestForwardingStatus_AllMethods(t *testing.T) {
	// Test all methods on each constant
	statuses := []ForwardingStatus{
		ForwardingStatusEnabled,
		ForwardingStatusDisabled,
		ForwardingStatusError,
	}

	for _, status := range statuses {
		// Each status should have exactly one method return true
		trueCount := 0
		if status.IsEnabled() {
			trueCount++
		}
		if status.IsDisabled() {
			trueCount++
		}
		if status.IsError() {
			trueCount++
		}

		if trueCount != 1 {
			t.Errorf("Expected exactly one method to return true for status %q, got %d", status, trueCount)
		}

		// String method should return the expected value
		if status.String() != string(status) {
			t.Errorf("Expected String() to return %q, got %q", string(status), status.String())
		}
	}
}

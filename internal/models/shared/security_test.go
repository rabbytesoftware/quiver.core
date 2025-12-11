package shared

import "testing"

func TestSecurity_String(t *testing.T) {
	testCases := []struct {
		name     string
		security Security
		expected string
	}{
		{
			name:     "trusted security",
			security: SecurityTrusted,
			expected: "trusted",
		},
		{
			name:     "untrusted security",
			security: SecurityUntrusted,
			expected: "untrusted",
		},
		{
			name:     "custom security",
			security: Security("custom"),
			expected: "custom",
		},
		{
			name:     "empty security",
			security: Security(""),
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.security.String()
			if result != tc.expected {
				t.Errorf("Expected String() to return %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestSecurity_IsTrusted(t *testing.T) {
	testCases := []struct {
		name     string
		security Security
		expected bool
	}{
		{
			name:     "trusted security",
			security: SecurityTrusted,
			expected: true,
		},
		{
			name:     "untrusted security",
			security: SecurityUntrusted,
			expected: false,
		},
		{
			name:     "custom security",
			security: Security("custom"),
			expected: false,
		},
		{
			name:     "empty security",
			security: Security(""),
			expected: false,
		},
		{
			name:     "uppercase trusted",
			security: Security("TRUSTED"),
			expected: false,
		},
		{
			name:     "mixed case trusted",
			security: Security("Trusted"),
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.security.IsTrusted()
			if result != tc.expected {
				t.Errorf("Expected IsTrusted() to return %v for %q, got %v", tc.expected, tc.security, result)
			}
		})
	}
}

func TestSecurity_IsUntrusted(t *testing.T) {
	testCases := []struct {
		name     string
		security Security
		expected bool
	}{
		{
			name:     "trusted security",
			security: SecurityTrusted,
			expected: false,
		},
		{
			name:     "untrusted security",
			security: SecurityUntrusted,
			expected: true,
		},
		{
			name:     "custom security",
			security: Security("custom"),
			expected: false,
		},
		{
			name:     "empty security",
			security: Security(""),
			expected: false,
		},
		{
			name:     "uppercase untrusted",
			security: Security("UNTRUSTED"),
			expected: false,
		},
		{
			name:     "mixed case untrusted",
			security: Security("Untrusted"),
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.security.IsUntrusted()
			if result != tc.expected {
				t.Errorf("Expected IsUntrusted() to return %v for %q, got %v", tc.expected, tc.security, result)
			}
		})
	}
}

func TestSecurity_Constants(t *testing.T) {
	// Test that constants have expected values
	if SecurityTrusted != "trusted" {
		t.Errorf("Expected SecurityTrusted to be 'trusted', got %q", SecurityTrusted)
	}

	if SecurityUntrusted != "untrusted" {
		t.Errorf("Expected SecurityUntrusted to be 'untrusted', got %q", SecurityUntrusted)
	}
}

func TestSecurity_AllMethods(t *testing.T) {
	// Test all methods on each constant
	securities := []Security{
		SecurityTrusted,
		SecurityUntrusted,
	}

	for _, security := range securities {
		// Each security should have exactly one method return true
		trueCount := 0
		if security.IsTrusted() {
			trueCount++
		}
		if security.IsUntrusted() {
			trueCount++
		}

		if trueCount != 1 {
			t.Errorf("Expected exactly one method to return true for security %q, got %d", security, trueCount)
		}

		// String method should return the expected value
		if security.String() != string(security) {
			t.Errorf("Expected String() to return %q, got %q", string(security), security.String())
		}
	}
}

func TestSecurity_TypeConversion(t *testing.T) {
	// Test that Security can be created from string
	str := "trusted"
	security := Security(str)

	if security.String() != str {
		t.Errorf("Expected security string to be %q, got %q", str, security.String())
	}

	// Test that it maintains the original string
	if string(security) != str {
		t.Errorf("Expected string conversion to be %q, got %q", str, string(security))
	}
}

func TestSecurity_EdgeCases(t *testing.T) {
	// Test with special characters
	security := Security("trusted!")
	if security.IsTrusted() {
		t.Error("Expected security with special characters to not be trusted")
	}

	// Test with whitespace
	security = Security(" trusted ")
	if security.IsTrusted() {
		t.Error("Expected security with whitespace to not be trusted")
	}

	// Test with unicode characters
	security = Security("信任")
	if security.IsTrusted() {
		t.Error("Expected unicode security to not be trusted")
	}

	// Test very long string
	longString := make([]byte, 1000)
	for i := range longString {
		longString[i] = 'a'
	}
	security = Security(string(longString))
	if security.IsTrusted() || security.IsUntrusted() {
		t.Error("Expected very long security string to not match any constant")
	}
}

func TestSecurity_MutualExclusivity(t *testing.T) {
	// Test that trusted and untrusted are mutually exclusive
	if SecurityTrusted.IsTrusted() && SecurityTrusted.IsUntrusted() {
		t.Error("SecurityTrusted should not be both trusted and untrusted")
	}

	if SecurityUntrusted.IsTrusted() && SecurityUntrusted.IsUntrusted() {
		t.Error("SecurityUntrusted should not be both trusted and untrusted")
	}

	// Test that each constant has exactly one true method
	if !SecurityTrusted.IsTrusted() {
		t.Error("SecurityTrusted should be trusted")
	}

	if SecurityTrusted.IsUntrusted() {
		t.Error("SecurityTrusted should not be untrusted")
	}

	if SecurityUntrusted.IsTrusted() {
		t.Error("SecurityUntrusted should not be trusted")
	}

	if !SecurityUntrusted.IsUntrusted() {
		t.Error("SecurityUntrusted should be untrusted")
	}
}

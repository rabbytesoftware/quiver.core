package domain

import "testing"

func TestNamespace_Validate(t *testing.T) {
	testCases := []struct {
		name        string
		namespace   Namespace
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid namespace",
			namespace:   Namespace("quiver:arrow"),
			expectError: false,
		},
		{
			name:        "empty namespace",
			namespace:   Namespace(""),
			expectError: true,
			errorMsg:    "namespace cannot be empty",
		},
		{
			name:        "missing AUID",
			namespace:   Namespace("quiver"),
			expectError: true,
			errorMsg:    "namespace must be in format QUID:AUID",
		},
		{
			name:        "empty QUID",
			namespace:   Namespace(":arrow"),
			expectError: true,
			errorMsg:    "QUID part of namespace cannot be empty",
		},
		{
			name:        "empty AUID",
			namespace:   Namespace("quiver:"),
			expectError: true,
			errorMsg:    "AUID part of namespace cannot be empty",
		},
		{
			name:        "multiple colons",
			namespace:   Namespace("quiver:arrow:extra"),
			expectError: true,
			errorMsg:    "namespace must be in format QUID:AUID",
		},
		{
			name:        "valid with complex names",
			namespace:   Namespace("my-quiver:my-arrow-123"),
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.namespace.Validate()
			if tc.expectError {
				if err == nil {
					t.Error("Expected error but got nil")
				} else if tc.errorMsg != "" && !namespaceContains(err.Error(), tc.errorMsg) {
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

func TestNamespace_GetQUID(t *testing.T) {
	testCases := []struct {
		name      string
		namespace Namespace
		expected  string
	}{
		{
			name:      "valid namespace",
			namespace: Namespace("quiver:arrow"),
			expected:  "quiver",
		},
		{
			name:      "empty namespace",
			namespace: Namespace(""),
			expected:  "",
		},
		{
			name:      "only QUID",
			namespace: Namespace("quiver"),
			expected:  "quiver",
		},
		{
			name:      "complex QUID",
			namespace: Namespace("my-quiver-123:arrow"),
			expected:  "my-quiver-123",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.namespace.GetQUID()
			if result != tc.expected {
				t.Errorf("Expected GetQUID() to return %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestNamespace_GetAUID(t *testing.T) {
	testCases := []struct {
		name      string
		namespace Namespace
		expected  string
	}{
		{
			name:      "valid namespace",
			namespace: Namespace("quiver:arrow"),
			expected:  "arrow",
		},
		{
			name:      "empty namespace",
			namespace: Namespace(""),
			expected:  "",
		},
		{
			name:      "only QUID",
			namespace: Namespace("quiver"),
			expected:  "",
		},
		{
			name:      "complex AUID",
			namespace: Namespace("quiver:my-arrow-123"),
			expected:  "my-arrow-123",
		},
		{
			name:      "multiple parts",
			namespace: Namespace("quiver:arrow:extra"),
			expected:  "arrow",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.namespace.GetAUID()
			if result != tc.expected {
				t.Errorf("Expected GetAUID() to return %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestNamespace_String(t *testing.T) {
	testCases := []struct {
		name      string
		namespace Namespace
		expected  string
	}{
		{
			name:      "valid namespace",
			namespace: Namespace("quiver:arrow"),
			expected:  "quiver:arrow",
		},
		{
			name:      "empty namespace",
			namespace: Namespace(""),
			expected:  "",
		},
		{
			name:      "complex namespace",
			namespace: Namespace("my-quiver:my-arrow"),
			expected:  "my-quiver:my-arrow",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.namespace.String()
			if result != tc.expected {
				t.Errorf("Expected String() to return %q, got %q", tc.expected, result)
			}
		})
	}
}

func namespaceContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

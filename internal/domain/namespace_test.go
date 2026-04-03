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
			name:        "valid standalone namespace",
			namespace:   Namespace("github.com/valve/steamcmd"),
			expectError: false,
		},
		{
			name:        "valid quiver-hosted namespace",
			namespace:   Namespace("github.com/char2cs/gaming.quiver/cs2"),
			expectError: false,
		},
		{
			name:        "empty namespace",
			namespace:   Namespace(""),
			expectError: true,
			errorMsg:    "namespace cannot be empty",
		},
		{
			name:        "single segment",
			namespace:   Namespace("github.com"),
			expectError: true,
			errorMsg:    "namespace must be in format",
		},
		{
			name:        "two segments",
			namespace:   Namespace("github.com/valve"),
			expectError: true,
			errorMsg:    "namespace must be in format",
		},
		{
			name:        "five segments",
			namespace:   Namespace("github.com/valve/repo/arrow/extra"),
			expectError: true,
			errorMsg:    "namespace must be in format",
		},
		{
			name:        "empty segment",
			namespace:   Namespace("github.com//steamcmd"),
			expectError: true,
			errorMsg:    "segment 1 cannot be empty",
		},
		{
			name:        "valid with complex names",
			namespace:   Namespace("gitlab.com/my-org/my-quiver/my-arrow-123"),
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
			name:      "standalone namespace",
			namespace: Namespace("github.com/valve/steamcmd"),
			expected:  "github.com/valve/steamcmd",
		},
		{
			name:      "quiver-hosted namespace",
			namespace: Namespace("github.com/char2cs/gaming.quiver/cs2"),
			expected:  "github.com/char2cs/gaming.quiver",
		},
		{
			name:      "empty namespace",
			namespace: Namespace(""),
			expected:  "",
		},
		{
			name:      "single segment",
			namespace: Namespace("github.com"),
			expected:  "github.com",
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
			name:      "standalone namespace",
			namespace: Namespace("github.com/valve/steamcmd"),
			expected:  "",
		},
		{
			name:      "quiver-hosted namespace",
			namespace: Namespace("github.com/char2cs/gaming.quiver/cs2"),
			expected:  "cs2",
		},
		{
			name:      "empty namespace",
			namespace: Namespace(""),
			expected:  "",
		},
		{
			name:      "complex AUID",
			namespace: Namespace("github.com/char2cs/gaming.quiver/my-arrow-123"),
			expected:  "my-arrow-123",
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

func TestNamespace_IsQuiverHosted(t *testing.T) {
	testCases := []struct {
		name      string
		namespace Namespace
		expected  bool
	}{
		{
			name:      "standalone namespace",
			namespace: Namespace("github.com/valve/steamcmd"),
			expected:  false,
		},
		{
			name:      "quiver-hosted namespace",
			namespace: Namespace("github.com/char2cs/gaming.quiver/cs2"),
			expected:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.namespace.IsQuiverHosted()
			if result != tc.expected {
				t.Errorf("Expected IsQuiverHosted() to return %v, got %v", tc.expected, result)
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
			name:      "standalone namespace",
			namespace: Namespace("github.com/valve/steamcmd"),
			expected:  "github.com/valve/steamcmd",
		},
		{
			name:      "empty namespace",
			namespace: Namespace(""),
			expected:  "",
		},
		{
			name:      "quiver-hosted namespace",
			namespace: Namespace("github.com/char2cs/gaming.quiver/cs2"),
			expected:  "github.com/char2cs/gaming.quiver/cs2",
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

func TestNamespace_Domain(t *testing.T) {
	testCases := []struct {
		name      string
		namespace Namespace
		expected  string
	}{
		{
			name:      "github namespace",
			namespace: Namespace("github.com/valve/steamcmd"),
			expected:  "github.com",
		},
		{
			name:      "gitlab namespace",
			namespace: Namespace("gitlab.com/org/repo/arrow"),
			expected:  "gitlab.com",
		},
		{
			name:      "custom domain",
			namespace: Namespace("gitea.example.org/user/project"),
			expected:  "gitea.example.org",
		},
		{
			name:      "empty namespace",
			namespace: Namespace(""),
			expected:  "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.namespace.Domain()
			if result != tc.expected {
				t.Errorf("Expected Domain() to return %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestNamespace_CloneURL(t *testing.T) {
	testCases := []struct {
		name      string
		namespace Namespace
		expected  string
	}{
		{
			name:      "standalone namespace",
			namespace: Namespace("github.com/valve/steamcmd"),
			expected:  "https://github.com/valve/steamcmd",
		},
		{
			name:      "quiver-hosted namespace",
			namespace: Namespace("github.com/char2cs/gaming.quiver/cs2"),
			expected:  "https://github.com/char2cs/gaming.quiver",
		},
		{
			name:      "custom domain",
			namespace: Namespace("gitlab.com/org/repo"),
			expected:  "https://gitlab.com/org/repo",
		},
		{
			name:      "empty namespace",
			namespace: Namespace(""),
			expected:  "",
		},
		{
			name:      "single segment",
			namespace: Namespace("github.com"),
			expected:  "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.namespace.CloneURL()
			if result != tc.expected {
				t.Errorf("Expected CloneURL() to return %q, got %q", tc.expected, result)
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

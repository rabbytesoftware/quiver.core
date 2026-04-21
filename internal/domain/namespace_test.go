package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
				require.Error(t, err)
				if tc.errorMsg != "" {
					assert.Contains(t, err.Error(), tc.errorMsg)
				}
			} else {
				assert.NoError(t, err)
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
			assert.Equal(t, tc.expected, result)
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
			assert.Equal(t, tc.expected, result)
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
			assert.Equal(t, tc.expected, result)
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
			assert.Equal(t, tc.expected, result)
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
			assert.Equal(t, tc.expected, result)
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
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestNamespace_BareNamespace(t *testing.T) {
	testCases := []struct {
		name      string
		namespace Namespace
		expected  Namespace
	}{
		{
			name:      "no ref",
			namespace: Namespace("github.com/valve/steamcmd"),
			expected:  Namespace("github.com/valve/steamcmd"),
		},
		{
			name:      "with version ref",
			namespace: Namespace("github.com/valve/steamcmd@v1.2.3"),
			expected:  Namespace("github.com/valve/steamcmd"),
		},
		{
			name:      "with glob ref",
			namespace: Namespace("github.com/valve/steamcmd@v1.*"),
			expected:  Namespace("github.com/valve/steamcmd"),
		},
		{
			name:      "empty namespace",
			namespace: Namespace(""),
			expected:  Namespace(""),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.namespace.BareNamespace()
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestNamespace_Ref(t *testing.T) {
	testCases := []struct {
		name      string
		namespace Namespace
		expected  string
	}{
		{
			name:      "no ref",
			namespace: Namespace("github.com/valve/steamcmd"),
			expected:  "",
		},
		{
			name:      "with version ref",
			namespace: Namespace("github.com/valve/steamcmd@v1.2.3"),
			expected:  "v1.2.3",
		},
		{
			name:      "with glob ref",
			namespace: Namespace("github.com/valve/steamcmd@v1.*"),
			expected:  "v1.*",
		},
		{
			name:      "empty namespace",
			namespace: Namespace(""),
			expected:  "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.namespace.Ref()
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestNamespace_IsGlob(t *testing.T) {
	testCases := []struct {
		name      string
		namespace Namespace
		expected  bool
	}{
		{
			name:      "no ref",
			namespace: Namespace("github.com/valve/steamcmd"),
			expected:  false,
		},
		{
			name:      "exact version ref",
			namespace: Namespace("github.com/valve/steamcmd@v1.2.3"),
			expected:  false,
		},
		{
			name:      "glob ref",
			namespace: Namespace("github.com/valve/steamcmd@v1.*"),
			expected:  true,
		},
		{
			name:      "wildcard-only ref",
			namespace: Namespace("github.com/valve/steamcmd@*"),
			expected:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.namespace.IsGlob()
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestNamespace_WithRef_ExistingMethods(t *testing.T) {
	ns := Namespace("github.com/valve/steamcmd@v1.2.3")

	t.Run("Validate with ref", func(t *testing.T) {
		assert.NoError(t, ns.Validate())
	})

	t.Run("GetQUID with ref", func(t *testing.T) {
		assert.Equal(t, "github.com/valve/steamcmd", ns.GetQUID())
	})

	t.Run("GetAUID with ref", func(t *testing.T) {
		assert.Equal(t, "", ns.GetAUID())
	})

	t.Run("IsQuiverHosted with ref", func(t *testing.T) {
		assert.False(t, ns.IsQuiverHosted())
	})

	t.Run("Domain with ref", func(t *testing.T) {
		assert.Equal(t, "github.com", ns.Domain())
	})

	t.Run("CloneURL with ref", func(t *testing.T) {
		assert.Equal(t, "https://github.com/valve/steamcmd", ns.CloneURL())
	})

	t.Run("String keeps ref", func(t *testing.T) {
		assert.Equal(t, "github.com/valve/steamcmd@v1.2.3", ns.String())
	})
}

func TestNamespace_WithRef(t *testing.T) {
	testCases := []struct {
		name      string
		namespace Namespace
		ref       string
		expected  Namespace
	}{
		{
			name:      "no existing ref, non-empty ref appended",
			namespace: Namespace("github.com/valve/steamcmd"),
			ref:       "v1.2.3",
			expected:  Namespace("github.com/valve/steamcmd@v1.2.3"),
		},
		{
			name:      "existing exact ref replaced by new ref",
			namespace: Namespace("github.com/valve/steamcmd@v1.0.0"),
			ref:       "v2.0.0",
			expected:  Namespace("github.com/valve/steamcmd@v2.0.0"),
		},
		{
			name:      "existing glob ref replaced by new ref",
			namespace: Namespace("github.com/valve/steamcmd@v1.*"),
			ref:       "v1.5.0",
			expected:  Namespace("github.com/valve/steamcmd@v1.5.0"),
		},
		{
			name:      "any ref plus empty string returns bare namespace",
			namespace: Namespace("github.com/valve/steamcmd@v1.2.3"),
			ref:       "",
			expected:  Namespace("github.com/valve/steamcmd"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.namespace.WithRef(tc.ref)
			assert.Equal(t, tc.expected, result)
		})
	}
}

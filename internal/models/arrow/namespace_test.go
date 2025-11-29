package arrow

import "testing"

func TestArrowNamespace_IsValid(t *testing.T) {
	testCases := []struct {
		name      string
		namespace ArrowNamespace
		expected  bool
	}{
		{
			name:      "valid namespace",
			namespace: ArrowNamespace("user@domain"),
			expected:  true,
		},
		{
			name:      "valid namespace with numbers",
			namespace: ArrowNamespace("user123@domain456"),
			expected:  true,
		},
		{
			name:      "empty namespace",
			namespace: ArrowNamespace(""),
			expected:  false,
		},
		{
			name:      "no @ symbol",
			namespace: ArrowNamespace("userdomain"),
			expected:  false,
		},
		{
			name:      "multiple @ symbols",
			namespace: ArrowNamespace("user@domain@extra"),
			expected:  false,
		},
		{
			name:      "empty user part",
			namespace: ArrowNamespace("@domain"),
			expected:  false,
		},
		{
			name:      "empty domain part",
			namespace: ArrowNamespace("user@"),
			expected:  false,
		},
		{
			name:      "only @ symbol",
			namespace: ArrowNamespace("@"),
			expected:  false,
		},
		{
			name:      "valid with special characters",
			namespace: ArrowNamespace("user-name@domain.com"),
			expected:  true,
		},
		{
			name:      "valid with underscores",
			namespace: ArrowNamespace("user_name@domain_name"),
			expected:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.namespace.IsValid()
			if result != tc.expected {
				t.Errorf("Expected IsValid() to return %v for %q, got %v", tc.expected, tc.namespace, result)
			}
		})
	}
}

func TestArrowNamespace_String(t *testing.T) {
	testCases := []struct {
		name      string
		namespace ArrowNamespace
		expected  string
	}{
		{
			name:      "simple namespace",
			namespace: ArrowNamespace("user@domain"),
			expected:  "user@domain",
		},
		{
			name:      "empty namespace",
			namespace: ArrowNamespace(""),
			expected:  "",
		},
		{
			name:      "complex namespace",
			namespace: ArrowNamespace("complex-user_123@sub.domain.com"),
			expected:  "complex-user_123@sub.domain.com",
		},
		{
			name:      "namespace with special chars",
			namespace: ArrowNamespace("user@domain!@#$"),
			expected:  "user@domain!@#$",
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

func TestArrowNamespace_TypeConversion(t *testing.T) {
	// Test that ArrowNamespace can be created from string
	str := "test@example.com"
	namespace := ArrowNamespace(str)

	if namespace.String() != str {
		t.Errorf("Expected namespace string to be %q, got %q", str, namespace.String())
	}

	// Test that it maintains the original string
	if string(namespace) != str {
		t.Errorf("Expected string conversion to be %q, got %q", str, string(namespace))
	}
}

func TestArrowNamespace_EdgeCases(t *testing.T) {
	// Test with whitespace
	namespace := ArrowNamespace(" user @ domain ")
	if !namespace.IsValid() {
		t.Error("Expected namespace with spaces to be valid (spaces are allowed)")
	}

	// Test with unicode characters
	namespace = ArrowNamespace("用户@域名")
	if !namespace.IsValid() {
		t.Error("Expected unicode namespace to be valid")
	}

	// Test very long namespace
	longUser := make([]byte, 1000)
	for i := range longUser {
		longUser[i] = 'a'
	}
	longDomain := make([]byte, 1000)
	for i := range longDomain {
		longDomain[i] = 'b'
	}
	longNamespace := ArrowNamespace(string(longUser) + "@" + string(longDomain))
	if !longNamespace.IsValid() {
		t.Error("Expected very long namespace to be valid")
	}
}

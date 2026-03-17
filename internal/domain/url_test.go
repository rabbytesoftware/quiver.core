package domain

import "testing"

func TestURL_String(t *testing.T) {
	testCases := []struct {
		name     string
		url      URL
		expected string
	}{
		{
			name:     "simple HTTP URL",
			url:      URL("http://example.com"),
			expected: "http://example.com",
		},
		{
			name:     "HTTPS URL with path",
			url:      URL("https://example.com/path"),
			expected: "https://example.com/path",
		},
		{
			name:     "FTP URL",
			url:      URL("ftp://files.example.com"),
			expected: "ftp://files.example.com",
		},
		{
			name:     "empty URL",
			url:      URL(""),
			expected: "",
		},
		{
			name:     "custom string",
			url:      URL("not-a-url"),
			expected: "not-a-url",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.url.String()
			if result != tc.expected {
				t.Errorf("Expected String() to return %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestURL_IsValid(t *testing.T) {
	testCases := []struct {
		name     string
		url      URL
		expected bool
	}{
		// Valid URLs
		{
			name:     "simple HTTP URL",
			url:      URL("http://example.com"),
			expected: true,
		},
		{
			name:     "simple HTTPS URL",
			url:      URL("https://example.com"),
			expected: true,
		},
		{
			name:     "FTP URL",
			url:      URL("ftp://files.example.com"),
			expected: true,
		},
		{
			name:     "HTTP URL with port",
			url:      URL("http://example.com:8080"),
			expected: true,
		},
		{
			name:     "HTTPS URL with path",
			url:      URL("https://example.com/path/to/resource"),
			expected: true,
		},
		{
			name:     "URL with query parameters",
			url:      URL("https://example.com/search?q=test&limit=10"),
			expected: true,
		},
		{
			name:     "URL with fragment",
			url:      URL("https://example.com/page#section"),
			expected: true,
		},
		{
			name:     "URL with subdomain",
			url:      URL("https://api.example.com"),
			expected: true,
		},
		{
			name:     "URL with IP address",
			url:      URL("http://192.168.1.1"),
			expected: true,
		},
		{
			name:     "URL with IP and port",
			url:      URL("http://192.168.1.1:3000"),
			expected: true,
		},
		{
			name:     "complex URL",
			url:      URL("https://user:pass@api.example.com:443/v1/users?active=true&sort=name#results"),
			expected: true,
		},

		// Invalid URLs
		{
			name:     "empty URL",
			url:      URL(""),
			expected: false,
		},
		{
			name:     "no protocol",
			url:      URL("example.com"),
			expected: false,
		},
		{
			name:     "invalid protocol",
			url:      URL("invalid://example.com"),
			expected: false,
		},
		{
			name:     "missing domain",
			url:      URL("http://"),
			expected: false,
		},
		{
			name:     "just protocol",
			url:      URL("http"),
			expected: false,
		},
		{
			name:     "malformed URL",
			url:      URL("http:/example.com"),
			expected: false,
		},
		{
			name:     "spaces in URL",
			url:      URL("http://example .com"),
			expected: false,
		},
		{
			name:     "URL with spaces",
			url:      URL("http://example.com/path with spaces"),
			expected: false,
		},
		{
			name:     "not a URL",
			url:      URL("just some text"),
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.url.IsValid()
			if result != tc.expected {
				t.Errorf("Expected IsValid() to return %v for %q, got %v", tc.expected, tc.url, result)
			}
		})
	}
}

func TestURL_TypeConversion(t *testing.T) {
	// Test that URL can be created from string
	str := "https://example.com"
	url := URL(str)

	if url.String() != str {
		t.Errorf("Expected URL string to be %q, got %q", str, url.String())
	}

	// Test that it maintains the original string
	if string(url) != str {
		t.Errorf("Expected string conversion to be %q, got %q", str, string(url))
	}
}

func TestURL_EdgeCases(t *testing.T) {
	// Test with very long URL
	longPath := make([]byte, 1000)
	for i := range longPath {
		longPath[i] = 'a'
	}
	longURL := URL("https://example.com/" + string(longPath))
	if !longURL.IsValid() {
		t.Error("Expected very long URL to be valid")
	}

	// Test with unicode characters in domain (the regex actually accepts these)
	unicodeURL := URL("https://\u4f8b\u3048.\u30c6\u30b9\u30c8")
	if !unicodeURL.IsValid() {
		t.Error("Expected unicode domain URL to be valid with current regex")
	}

	// Test with localhost
	localhostURL := URL("http://localhost:3000")
	if !localhostURL.IsValid() {
		t.Error("Expected localhost URL to be valid")
	}

	// Test with file protocol (should be invalid)
	fileURL := URL("file:///path/to/file")
	if fileURL.IsValid() {
		t.Error("Expected file protocol URL to be invalid")
	}
}

func TestURL_ProtocolVariations(t *testing.T) {
	// Test different valid protocols
	validProtocols := []string{"http", "https", "ftp"}

	for _, protocol := range validProtocols {
		url := URL(protocol + "://example.com")
		if !url.IsValid() {
			t.Errorf("Expected URL with %s protocol to be valid", protocol)
		}
	}

	// Test invalid protocols
	invalidProtocols := []string{"smtp", "file", "ws", "wss", "custom"}

	for _, protocol := range invalidProtocols {
		url := URL(protocol + "://example.com")
		if url.IsValid() {
			t.Errorf("Expected URL with %s protocol to be invalid", protocol)
		}
	}
}

func TestURL_DomainVariations(t *testing.T) {
	// Test different valid domain formats
	validDomains := []string{
		"example.com",
		"sub.example.com",
		"api.v1.example.com",
		"192.168.1.1",
		"10.0.0.1",
		"localhost",
		"example-site.com",
		"example123.com",
	}

	for _, domain := range validDomains {
		url := URL("https://" + domain)
		if !url.IsValid() {
			t.Errorf("Expected URL with domain %s to be valid", domain)
		}
	}
}

func TestURL_PathAndQueryVariations(t *testing.T) {
	baseURL := "https://example.com"

	// Test different valid path and query combinations
	validSuffixes := []string{
		"/",
		"/path",
		"/path/to/resource",
		"/path?query=value",
		"/path?q1=v1&q2=v2",
		"/path#fragment",
		"/path?query=value#fragment",
		"?query=value",
		"#fragment",
	}

	for _, suffix := range validSuffixes {
		url := URL(baseURL + suffix)
		if !url.IsValid() {
			t.Errorf("Expected URL %q to be valid", url)
		}
	}
}

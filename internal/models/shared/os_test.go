package shared

import "testing"

func TestOS_String(t *testing.T) {
	testCases := []struct {
		name     string
		os       OS
		expected string
	}{
		{
			name:     "Linux AMD64",
			os:       OSLinuxAMD64,
			expected: "linux/amd64",
		},
		{
			name:     "Linux ARM64",
			os:       OSLinuxARM64,
			expected: "linux/arm64",
		},
		{
			name:     "Windows AMD64",
			os:       OSWindowsAMD64,
			expected: "windows/amd64",
		},
		{
			name:     "Windows ARM64",
			os:       OSWindowsARM64,
			expected: "windows/arm64",
		},
		{
			name:     "Darwin AMD64",
			os:       OSDarwinAMD64,
			expected: "darwin/amd64",
		},
		{
			name:     "Darwin ARM64",
			os:       OSDarwinARM64,
			expected: "darwin/arm64",
		},
		{
			name:     "custom OS",
			os:       OS("custom/arch"),
			expected: "custom/arch",
		},
		{
			name:     "empty OS",
			os:       OS(""),
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.os.String()
			if result != tc.expected {
				t.Errorf("Expected String() to return %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestOS_IsValid(t *testing.T) {
	testCases := []struct {
		name     string
		os       OS
		expected bool
	}{
		{
			name:     "Linux AMD64",
			os:       OSLinuxAMD64,
			expected: true,
		},
		{
			name:     "Linux ARM64",
			os:       OSLinuxARM64,
			expected: true,
		},
		{
			name:     "Windows AMD64",
			os:       OSWindowsAMD64,
			expected: true,
		},
		{
			name:     "Windows ARM64",
			os:       OSWindowsARM64,
			expected: true,
		},
		{
			name:     "Darwin AMD64",
			os:       OSDarwinAMD64,
			expected: true,
		},
		{
			name:     "Darwin ARM64",
			os:       OSDarwinARM64,
			expected: true,
		},
		{
			name:     "custom OS",
			os:       OS("custom/arch"),
			expected: false,
		},
		{
			name:     "empty OS",
			os:       OS(""),
			expected: false,
		},
		{
			name:     "invalid format",
			os:       OS("linux"),
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.os.IsValid()
			if result != tc.expected {
				t.Errorf("Expected IsValid() to return %v for %q, got %v", tc.expected, tc.os, result)
			}
		})
	}
}

func TestOS_IsLinux(t *testing.T) {
	testCases := []struct {
		name     string
		os       OS
		expected bool
	}{
		{
			name:     "Linux AMD64",
			os:       OSLinuxAMD64,
			expected: true,
		},
		{
			name:     "Linux ARM64",
			os:       OSLinuxARM64,
			expected: true,
		},
		{
			name:     "Windows AMD64",
			os:       OSWindowsAMD64,
			expected: false,
		},
		{
			name:     "Darwin AMD64",
			os:       OSDarwinAMD64,
			expected: false,
		},
		{
			name:     "custom OS",
			os:       OS("custom"),
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.os.IsLinux()
			if result != tc.expected {
				t.Errorf("Expected IsLinux() to return %v for %q, got %v", tc.expected, tc.os, result)
			}
		})
	}
}

func TestOS_IsWindows(t *testing.T) {
	testCases := []struct {
		name     string
		os       OS
		expected bool
	}{
		{
			name:     "Windows AMD64",
			os:       OSWindowsAMD64,
			expected: true,
		},
		{
			name:     "Windows ARM64",
			os:       OSWindowsARM64,
			expected: true,
		},
		{
			name:     "Linux AMD64",
			os:       OSLinuxAMD64,
			expected: false,
		},
		{
			name:     "Darwin AMD64",
			os:       OSDarwinAMD64,
			expected: false,
		},
		{
			name:     "custom OS",
			os:       OS("custom"),
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.os.IsWindows()
			if result != tc.expected {
				t.Errorf("Expected IsWindows() to return %v for %q, got %v", tc.expected, tc.os, result)
			}
		})
	}
}

func TestOS_IsDarwin(t *testing.T) {
	testCases := []struct {
		name     string
		os       OS
		expected bool
	}{
		{
			name:     "Darwin AMD64",
			os:       OSDarwinAMD64,
			expected: true,
		},
		{
			name:     "Darwin ARM64",
			os:       OSDarwinARM64,
			expected: true,
		},
		{
			name:     "Linux AMD64",
			os:       OSLinuxAMD64,
			expected: false,
		},
		{
			name:     "Windows AMD64",
			os:       OSWindowsAMD64,
			expected: false,
		},
		{
			name:     "custom OS",
			os:       OS("custom"),
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.os.IsDarwin()
			if result != tc.expected {
				t.Errorf("Expected IsDarwin() to return %v for %q, got %v", tc.expected, tc.os, result)
			}
		})
	}
}

func TestOS_IsAMD64(t *testing.T) {
	testCases := []struct {
		name     string
		os       OS
		expected bool
	}{
		{
			name:     "Linux AMD64",
			os:       OSLinuxAMD64,
			expected: true,
		},
		{
			name:     "Windows AMD64",
			os:       OSWindowsAMD64,
			expected: true,
		},
		{
			name:     "Darwin AMD64",
			os:       OSDarwinAMD64,
			expected: true,
		},
		{
			name:     "Linux ARM64",
			os:       OSLinuxARM64,
			expected: false,
		},
		{
			name:     "Windows ARM64",
			os:       OSWindowsARM64,
			expected: false,
		},
		{
			name:     "Darwin ARM64",
			os:       OSDarwinARM64,
			expected: false,
		},
		{
			name:     "custom OS",
			os:       OS("custom"),
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.os.IsAMD64()
			if result != tc.expected {
				t.Errorf("Expected IsAMD64() to return %v for %q, got %v", tc.expected, tc.os, result)
			}
		})
	}
}

func TestOS_IsARM64(t *testing.T) {
	testCases := []struct {
		name     string
		os       OS
		expected bool
	}{
		{
			name:     "Linux ARM64",
			os:       OSLinuxARM64,
			expected: true,
		},
		{
			name:     "Windows ARM64",
			os:       OSWindowsARM64,
			expected: true,
		},
		{
			name:     "Darwin ARM64",
			os:       OSDarwinARM64,
			expected: true,
		},
		{
			name:     "Linux AMD64",
			os:       OSLinuxAMD64,
			expected: false,
		},
		{
			name:     "Windows AMD64",
			os:       OSWindowsAMD64,
			expected: false,
		},
		{
			name:     "Darwin AMD64",
			os:       OSDarwinAMD64,
			expected: false,
		},
		{
			name:     "custom OS",
			os:       OS("custom"),
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.os.IsARM64()
			if result != tc.expected {
				t.Errorf("Expected IsARM64() to return %v for %q, got %v", tc.expected, tc.os, result)
			}
		})
	}
}

func TestOS_Constants(t *testing.T) {
	// Test that constants have expected values
	expectedConstants := map[OS]string{
		OSLinuxAMD64:   "linux/amd64",
		OSLinuxARM64:   "linux/arm64",
		OSWindowsAMD64: "windows/amd64",
		OSWindowsARM64: "windows/arm64",
		OSDarwinAMD64:  "darwin/amd64",
		OSDarwinARM64:  "darwin/arm64",
	}

	for osConst, expectedValue := range expectedConstants {
		if string(osConst) != expectedValue {
			t.Errorf("Expected constant %q to have value %q, got %q", osConst, expectedValue, string(osConst))
		}
	}
}

func TestOS_AllMethods(t *testing.T) {
	// Test all methods on each constant
	allOS := []OS{
		OSLinuxAMD64,
		OSLinuxARM64,
		OSWindowsAMD64,
		OSWindowsARM64,
		OSDarwinAMD64,
		OSDarwinARM64,
	}

	for _, os := range allOS {
		// All defined constants should be valid
		if !os.IsValid() {
			t.Errorf("Expected OS %q to be valid", os)
		}

		// Each OS should have exactly one OS type method return true
		osTypeCount := 0
		if os.IsLinux() {
			osTypeCount++
		}
		if os.IsWindows() {
			osTypeCount++
		}
		if os.IsDarwin() {
			osTypeCount++
		}

		if osTypeCount != 1 {
			t.Errorf("Expected exactly one OS type method to return true for %q, got %d", os, osTypeCount)
		}

		// Each OS should have exactly one architecture method return true
		archCount := 0
		if os.IsAMD64() {
			archCount++
		}
		if os.IsARM64() {
			archCount++
		}

		if archCount != 1 {
			t.Errorf("Expected exactly one architecture method to return true for %q, got %d", os, archCount)
		}

		// String method should return the expected value
		if os.String() != string(os) {
			t.Errorf("Expected String() to return %q, got %q", string(os), os.String())
		}
	}
}

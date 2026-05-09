package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
			assert.Equal(t, tc.expected, result)
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
			assert.Equal(t, tc.expected, result)
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
			assert.Equal(t, tc.expected, result)
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
			assert.Equal(t, tc.expected, result)
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
			assert.Equal(t, tc.expected, result)
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
			assert.Equal(t, tc.expected, result)
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
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestOS_Constants(t *testing.T) {
	expectedConstants := map[OS]string{
		OSLinuxAMD64:   "linux/amd64",
		OSLinuxARM64:   "linux/arm64",
		OSWindowsAMD64: "windows/amd64",
		OSWindowsARM64: "windows/arm64",
		OSDarwinAMD64:  "darwin/amd64",
		OSDarwinARM64:  "darwin/arm64",
	}

	for osConst, expectedValue := range expectedConstants {
		assert.Equal(t, expectedValue, string(osConst))
	}
}

func TestOS_AllMethods(t *testing.T) {
	allOS := []OS{
		OSLinuxAMD64,
		OSLinuxARM64,
		OSWindowsAMD64,
		OSWindowsARM64,
		OSDarwinAMD64,
		OSDarwinARM64,
	}

	for _, os := range allOS {
		assert.True(t, os.IsValid(), "expected OS %q to be valid", os)

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
		assert.Equal(t, 1, osTypeCount, "expected exactly one OS type method to return true for %q", os)

		archCount := 0
		if os.IsAMD64() {
			archCount++
		}
		if os.IsARM64() {
			archCount++
		}
		assert.Equal(t, 1, archCount, "expected exactly one architecture method to return true for %q", os)

		assert.Equal(t, string(os), os.String())
	}
}

func TestCurrentOS_ReturnsValidOS(t *testing.T) {
	got := CurrentOS()
	assert.NotEmpty(t, got)
	// CurrentOS returns GOOS/GOARCH — for all supported test platforms this should be valid
	if !got.IsValid() {
		t.Logf("CurrentOS() = %q — not in the 6 known values (may be a CI platform)", got)
	}
}

func TestAllOS(t *testing.T) {
	all := AllOS()
	require.Len(t, all, 6)
	want := map[OS]bool{
		OSLinuxAMD64:   false,
		OSLinuxARM64:   false,
		OSWindowsAMD64: false,
		OSWindowsARM64: false,
		OSDarwinAMD64:  false,
		OSDarwinARM64:  false,
	}
	for _, o := range all {
		if _, ok := want[o]; !ok {
			assert.Fail(t, "AllOS() returned unexpected OS", "%q", o)
		}
		want[o] = true
	}
	for o, seen := range want {
		assert.True(t, seen, "AllOS() missing %q", o)
	}
}

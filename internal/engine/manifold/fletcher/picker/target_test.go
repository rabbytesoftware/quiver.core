package picker

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestTarget_TargetOf_AllOS(t *testing.T) {
	testCases := []struct {
		name string
		os   domain.OS
		want target
	}{
		{name: "linux amd64", os: domain.OSLinuxAMD64, want: target{family: familyLinux, arch: archAMD64}},
		{name: "linux arm64", os: domain.OSLinuxARM64, want: target{family: familyLinux, arch: archARM64}},
		{name: "darwin amd64", os: domain.OSDarwinAMD64, want: target{family: familyDarwin, arch: archAMD64}},
		{name: "darwin arm64", os: domain.OSDarwinARM64, want: target{family: familyDarwin, arch: archARM64}},
		{name: "windows amd64", os: domain.OSWindowsAMD64, want: target{family: familyWindows, arch: archAMD64}},
		{name: "windows arm64", os: domain.OSWindowsARM64, want: target{family: familyWindows, arch: archARM64}},
		{name: "unknown", os: domain.OS("plan9/386"), want: target{}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, targetOf(tc.os))
		})
	}
}

func TestTarget_Contains_Family(t *testing.T) {
	tg := target{family: familyLinux, arch: archAMD64}

	assert.True(t, tg.contains(classification{family: familyLinux}))
	assert.False(t, tg.contains(classification{family: familyDarwin}))
}

func TestTarget_Native_Arches(t *testing.T) {
	testCases := []struct {
		name   string
		target target
		arch   string
		want   bool
	}{
		{name: "same arch", target: target{family: familyLinux, arch: archAMD64}, arch: archAMD64, want: true},
		{name: "other arch", target: target{family: familyLinux, arch: archAMD64}, arch: archARM64},
		{name: "archless", target: target{family: familyLinux, arch: archAMD64}, arch: archNone},
		{name: "universal", target: target{family: familyDarwin, arch: archARM64}, arch: archUniversal},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.target.native(classification{arch: tc.arch}))
		})
	}
}

func TestTarget_Universal_DarwinOnly(t *testing.T) {
	testCases := []struct {
		name   string
		target target
		arch   string
		want   bool
	}{
		{name: "universal on darwin", target: target{family: familyDarwin, arch: archARM64}, arch: archUniversal, want: true},
		{name: "universal on linux", target: target{family: familyLinux, arch: archARM64}, arch: archUniversal},
		{name: "universal on windows", target: target{family: familyWindows, arch: archAMD64}, arch: archUniversal},
		{name: "native on darwin", target: target{family: familyDarwin, arch: archARM64}, arch: archARM64},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.target.universal(classification{arch: tc.arch}))
		})
	}
}

func TestTarget_AssumesArch_LinuxARM64Never(t *testing.T) {
	testCases := []struct {
		name   string
		target target
		want   bool
	}{
		{name: "linux amd64", target: targetOf(domain.OSLinuxAMD64), want: true},
		{name: "linux arm64", target: targetOf(domain.OSLinuxARM64)},
		{name: "darwin arm64", target: targetOf(domain.OSDarwinARM64), want: true},
		{name: "windows arm64", target: targetOf(domain.OSWindowsARM64), want: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.target.assumesArch())
		})
	}
}

func TestTarget_Emulates_ARM64OffLinux(t *testing.T) {
	testCases := []struct {
		name   string
		target target
		want   bool
	}{
		{name: "linux amd64", target: targetOf(domain.OSLinuxAMD64)},
		{name: "linux arm64", target: targetOf(domain.OSLinuxARM64)},
		{name: "darwin amd64", target: targetOf(domain.OSDarwinAMD64)},
		{name: "darwin arm64", target: targetOf(domain.OSDarwinARM64), want: true},
		{name: "windows amd64", target: targetOf(domain.OSWindowsAMD64)},
		{name: "windows arm64", target: targetOf(domain.OSWindowsARM64), want: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.target.emulates())
		})
	}
}

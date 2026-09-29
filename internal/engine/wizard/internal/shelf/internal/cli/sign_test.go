package cli

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/platform"
)

func TestIsMachO(t *testing.T) {
	dir := t.TempDir()

	testCases := []struct {
		name  string
		bytes []byte
		want  bool
	}{
		{name: "64-bit little endian", bytes: []byte{0xcf, 0xfa, 0xed, 0xfe, 0}, want: true},
		{name: "32-bit little endian", bytes: []byte{0xce, 0xfa, 0xed, 0xfe}, want: true},
		{name: "64-bit big endian", bytes: []byte{0xfe, 0xed, 0xfa, 0xcf}, want: true},
		{name: "32-bit big endian", bytes: []byte{0xfe, 0xed, 0xfa, 0xce}, want: true},
		{name: "universal", bytes: []byte{0xca, 0xfe, 0xba, 0xbe}, want: true},
		{name: "elf", bytes: []byte{0x7f, 'E', 'L', 'F'}, want: false},
		{name: "short", bytes: []byte{0xcf}, want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.name)
			mocks.WriteFile(t, path, string(tc.bytes), 0o755)
			assert.Equal(t, tc.want, isMachO(path))
		})
	}

	assert.False(t, isMachO(filepath.Join(dir, "missing")))
}

func TestPlacer_Sign(t *testing.T) {
	dir := t.TempDir()
	machO := filepath.Join(dir, "tool")
	mocks.WriteFile(t, machO, string([]byte{0xcf, 0xfa, 0xed, 0xfe}), 0o755)
	script := filepath.Join(dir, "script")
	mocks.WriteFile(t, script, "#!/bin/sh\n", 0o755)
	inBundle := filepath.Join(dir, "Tool.app", "Contents", "MacOS", "tool")
	mocks.WriteFile(t, inBundle, string([]byte{0xcf, 0xfa, 0xed, 0xfe}), 0o755)
	boom := errors.New("boom")
	verify := []string{"codesign", "-v", machO}
	adhoc := []string{"codesign", "-s", "-", "-f", machO}

	testCases := []struct {
		name      string
		goos      string
		goarch    string
		workdir   string
		target    string
		verifyErr error
		signErr   error
		want      [][]string
	}{
		{name: "linux", goos: "linux", goarch: platform.GOARCHARM64, workdir: dir, target: machO},
		{name: "intel mac", goos: platform.GOOSDarwin, goarch: "amd64", workdir: dir, target: machO},
		{name: "not mach-o", goos: platform.GOOSDarwin, goarch: platform.GOARCHARM64, workdir: dir, target: script},
		{name: "inside a bundle", goos: platform.GOOSDarwin, goarch: platform.GOARCHARM64, workdir: dir, target: inBundle},
		{name: "outside the workdir", goos: platform.GOOSDarwin, goarch: platform.GOARCHARM64, workdir: filepath.Join(dir, "Tool.app"), target: machO},
		{name: "already signed", goos: platform.GOOSDarwin, goarch: platform.GOARCHARM64, workdir: dir, target: machO, want: [][]string{verify}},
		{name: "unsigned", goos: platform.GOOSDarwin, goarch: platform.GOARCHARM64, workdir: dir, target: machO, verifyErr: boom, want: [][]string{verify, adhoc}},
		{name: "signing fails", goos: platform.GOOSDarwin, goarch: platform.GOARCHARM64, workdir: dir, target: machO, verifyErr: boom, signErr: boom, want: [][]string{verify, adhoc}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, tc.goos)
			f.placer.host.GOARCH = tc.goarch
			f.Cmd.Respond = func(_ string, args, _ []string) ([]byte, error) {
				if args[0] == "-v" {
					return nil, tc.verifyErr
				}
				return []byte("out"), tc.signErr
			}

			f.placer.sign(context.Background(), tc.workdir, tc.target)

			assert.Equal(t, tc.want, f.Cmd.Calls)
		})
	}
}

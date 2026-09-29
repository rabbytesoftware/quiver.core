package cli

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/mocks"
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

func TestCodesign_Sign(t *testing.T) {
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
		workdir   string
		target    string
		verifyErr error
		signErr   error
		want      [][]string
	}{
		{name: "not mach-o", workdir: dir, target: script},
		{name: "inside a bundle", workdir: dir, target: inBundle},
		{name: "outside the workdir", workdir: filepath.Join(dir, "Tool.app"), target: machO},
		{name: "already signed", workdir: dir, target: machO, want: [][]string{verify}},
		{name: "unsigned", workdir: dir, target: machO, verifyErr: boom, want: [][]string{verify, adhoc}},
		{name: "signing fails", workdir: dir, target: machO, verifyErr: boom, signErr: boom, want: [][]string{verify, adhoc}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &mocks.Commander{Respond: func(_ string, args []string) ([]byte, error) {
				if args[0] == "-v" {
					return nil, tc.verifyErr
				}
				return []byte("out"), tc.signErr
			}}

			Codesign(cmd).Sign(context.Background(), tc.workdir, tc.target)

			assert.Equal(t, tc.want, cmd.Calls)
		})
	}
}

func TestUnsigned_SignsNothing(t *testing.T) {
	assert.NotPanics(t, func() { Unsigned().Sign(context.Background(), "/wd", "/wd/tool") })
}

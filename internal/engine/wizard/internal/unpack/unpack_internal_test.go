package unpack

import (
	"archive/tar"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/guard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/mocks"
)

func TestHostRules_ChosenOncePerOS(t *testing.T) {
	testCases := []struct {
		goos string
		want guard.HostRules
	}{
		{goos: "windows", want: guard.HostRules{WindowsNames: true, FoldCase: true, TypedLinks: true}},
		{goos: "darwin", want: guard.HostRules{FoldCase: true}},
		{goos: "linux", want: guard.HostRules{}},
		{goos: "freebsd", want: guard.HostRules{}},
	}

	for _, tc := range testCases {
		t.Run(tc.goos, func(t *testing.T) {
			assert.Equal(t, tc.want, hostRules(tc.goos))
		})
	}
}

func unpackTar(
	t *testing.T,
	goos string,
	entries ...mocks.TarEntry,
) error {
	t.Helper()

	path := mocks.WriteFile(t, filepath.Join(t.TempDir(), "a.tar"), mocks.TarBytes(t, entries...))
	src, err := os.Open(path)
	require.NoError(t, err)
	defer src.Close()
	info, err := src.Stat()
	require.NoError(t, err)

	format, err := newForOS(mocks.TestMaxBytes, goos).Detect(src, info.Size())
	require.NoError(t, err)

	_, err = format.Unpack(context.Background(), Target{Dir: filepath.Join(t.TempDir(), "out")})

	return err
}

func TestNewForOS_AppliesTheHostNameRules(t *testing.T) {
	reserved := mocks.TarEntry{Name: "bin/aux.txt", Body: "x", Mode: 0o644, Flag: tar.TypeReg}
	upper := mocks.TarEntry{Name: "README", Body: "x", Mode: 0o644, Flag: tar.TypeReg}
	lower := mocks.TarEntry{Name: "readme", Body: "y", Mode: 0o644, Flag: tar.TypeReg}

	require.ErrorIs(t, unpackTar(t, "windows", reserved), ErrReservedName)
	require.ErrorIs(t, unpackTar(t, "windows", upper, lower), ErrNameCollision)
	require.ErrorIs(t, unpackTar(t, "darwin", upper, lower), ErrNameCollision)
	require.NoError(t, unpackTar(t, "linux", upper, lower))
}

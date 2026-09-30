package guard_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/guard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/mocks"
)

func writeFile(
	g *guard.Guard,
	name string,
	body string,
) error {
	return g.File(context.Background(), name, 0o644, strings.NewReader(body))
}

func TestVerify_TypedLinks(t *testing.T) {
	testCases := []struct {
		name  string
		build func(g *guard.Guard) error
		check func(t *testing.T, dest string)
	}{
		{
			name: "link made before its directory target reads through",
			build: func(g *guard.Guard) error {
				return firstErr(g.Symlink("lib", "usr/lib"), writeFile(g, "usr/lib/x.so", "so"))
			},
			check: func(t *testing.T, dest string) {
				assert.Equal(t, "so", mocks.ReadString(t, filepath.Join(dest, "lib", "x.so")))
				target, err := os.Readlink(filepath.Join(dest, "lib"))
				require.NoError(t, err)
				assert.Equal(t, filepath.FromSlash("usr/lib"), target)
			},
		},
		{
			name: "nested link made before its directory target reads through",
			build: func(g *guard.Guard) error {
				return firstErr(g.Symlink("a/current", "../versions/2"), writeFile(g, "versions/2/bin", "v2"))
			},
			check: func(t *testing.T, dest string) {
				assert.Equal(t, "v2", mocks.ReadString(t, filepath.Join(dest, "a", "current", "bin")))
			},
		},
		{
			name: "link made before its file target reads through",
			build: func(g *guard.Guard) error {
				return firstErr(g.Symlink("run", "bin/tool"), writeFile(g, "bin/tool", "tool"))
			},
			check: func(t *testing.T, dest string) {
				assert.Equal(t, "tool", mocks.ReadString(t, filepath.Join(dest, "run")))
			},
		},
		{
			name: "link to an existing directory is left alone",
			build: func(g *guard.Guard) error {
				return firstErr(writeFile(g, "d/x", "x"), g.Symlink("l", "d"))
			},
			check: func(t *testing.T, dest string) {
				assert.Equal(t, "x", mocks.ReadString(t, filepath.Join(dest, "l", "x")))
			},
		},
		{
			name:  "dangling link stays dangling",
			build: func(g *guard.Guard) error { return g.Symlink("gone", "missing") },
			check: func(t *testing.T, dest string) {
				target, err := os.Readlink(filepath.Join(dest, "gone"))
				require.NoError(t, err)
				assert.Equal(t, "missing", target)
			},
		},
		{
			name: "link replaced by a file keeps the file",
			build: func(g *guard.Guard) error {
				return firstErr(g.Symlink("x", "d"), writeFile(g, "x", "file"), g.Dir("d", 0o755))
			},
			check: func(t *testing.T, dest string) {
				info, err := os.Lstat(filepath.Join(dest, "x"))
				require.NoError(t, err)
				assert.True(t, info.Mode().IsRegular())
				assert.Equal(t, "file", mocks.ReadString(t, filepath.Join(dest, "x")))
			},
		},
		{
			name: "re-pointed link keeps its last target",
			build: func(g *guard.Guard) error {
				return firstErr(g.Symlink("x", "old"), g.Symlink("x", "new"), g.Dir("old", 0o755), writeFile(g, "new/y", "y"))
			},
			check: func(t *testing.T, dest string) {
				target, err := os.Readlink(filepath.Join(dest, "x"))
				require.NoError(t, err)
				assert.Equal(t, "new", target)
				assert.Equal(t, "y", mocks.ReadString(t, filepath.Join(dest, "x", "y")))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g, dest := openGuard(t, guard.WithHostRules(guard.HostRules{TypedLinks: true}))

			require.NoError(t, tc.build(g))
			require.NoError(t, g.Verify())

			tc.check(t, dest)
		})
	}
}

func TestVerify_TypedLinksStillRejectEscapes(t *testing.T) {
	g, dest := openGuard(t, guard.WithHostRules(guard.HostRules{TypedLinks: true}))
	require.NoError(t, g.Dir("d", 0o755))
	require.NoError(t, g.Symlink("d/up", ".."))
	require.NoError(t, g.Symlink("esc", "d/up/.."))

	require.Error(t, g.Verify())

	_, err := os.Lstat(filepath.Join(dest, "esc"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestVerify_TypedLinkThatCannotBeReplacedFails(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("directory permissions do not block removal here")
	}
	g, dest := openGuard(t, guard.WithHostRules(guard.HostRules{TypedLinks: true}))
	require.NoError(t, g.Symlink("locked/l", "../target"))
	require.NoError(t, g.Dir("target", 0o755))
	locked := filepath.Join(dest, "locked")
	require.NoError(t, os.Chmod(locked, 0o500))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	require.ErrorIs(t, g.Verify(), fs.ErrPermission)
}

func firstErr(
	errs ...error,
) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}

	return nil
}

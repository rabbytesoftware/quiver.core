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
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/mocks"
)

func openGuard(
	t *testing.T,
	opts ...guard.Option,
) (*guard.Guard, string) {
	t.Helper()

	dest := filepath.Join(t.TempDir(), "out")
	g, err := guard.Open(context.Background(), dest, mocks.TestMaxBytes, opts...)
	require.NoError(t, err)
	t.Cleanup(g.Close)

	return g, dest
}

func sourceTree(
	t *testing.T,
) string {
	t.Helper()

	root := t.TempDir()
	mocks.WriteFile(t, filepath.Join(root, "keep", "a.txt"), []byte("a"))
	require.NoError(t, os.Symlink("a.txt", filepath.Join(root, "keep", "link")))
	mocks.WriteFile(t, filepath.Join(root, "skip", "x"), []byte("x"))
	mocks.WriteFile(t, filepath.Join(root, "flat", "y"), []byte("y"))
	mocks.WriteFile(t, filepath.Join(root, "drop.txt"), []byte("d"))

	return root
}

func flattenRoute(
	_ string,
	rel string,
	_ fs.DirEntry,
) (string, bool) {
	rel = filepath.ToSlash(rel)
	switch {
	case rel == "skip" || rel == "drop.txt":
		return "", false
	case rel == "flat":
		return "", true
	case strings.HasPrefix(rel, "flat/"):
		return strings.TrimPrefix(rel, "flat/"), true
	}

	return rel, true
}

func TestGuard_CopyTree_RoutesEntries(t *testing.T) {
	root := sourceTree(t)
	g, dest := openGuard(t)

	require.NoError(t, g.CopyTree(context.Background(), root, flattenRoute))
	require.NoError(t, g.Verify())

	assert.Equal(t, []string{"keep", "y"}, g.TopLevel())
	assert.Equal(t, "a", mocks.ReadString(t, filepath.Join(dest, "keep", "a.txt")))
	assert.Equal(t, "y", mocks.ReadString(t, filepath.Join(dest, "y")))
	target, err := os.Readlink(filepath.Join(dest, "keep", "link"))
	require.NoError(t, err)
	assert.Equal(t, "a.txt", target)
	for _, dropped := range []string{"skip", "drop.txt", "flat"} {
		_, err := os.Lstat(filepath.Join(dest, dropped))
		assert.ErrorIs(t, err, os.ErrNotExist, dropped)
	}
}

func TestGuard_CopyTree_Failures(t *testing.T) {
	testCases := []struct {
		name    string
		root    func(t *testing.T) string
		route   guard.Route
		wantErr error
	}{
		{
			name:    "missing root",
			root:    func(t *testing.T) string { return filepath.Join(t.TempDir(), "missing") },
			route:   flattenRoute,
			wantErr: fs.ErrNotExist,
		},
		{
			name: "route escapes the destination",
			root: sourceTree,
			route: func(_, rel string, _ fs.DirEntry) (string, bool) {
				return filepath.Join("..", rel), true
			},
			wantErr: models.ErrEscape,
		},
		{
			name: "escaping link in the tree",
			root: func(t *testing.T) string {
				root := t.TempDir()
				require.NoError(t, os.Symlink(filepath.Join("..", "..", "etc"), filepath.Join(root, "evil")))
				return root
			},
			route:   flattenRoute,
			wantErr: models.ErrEscape,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g, _ := openGuard(t)

			err := g.CopyTree(context.Background(), tc.root(t), tc.route)

			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestGuard_CopyTree_UnreadableFile(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permission bits do not block reads here")
	}
	root := t.TempDir()
	locked := mocks.WriteFile(t, filepath.Join(root, "locked"), []byte("x"))
	require.NoError(t, os.Chmod(locked, 0o000))
	g, _ := openGuard(t)

	err := g.CopyTree(context.Background(), root, flattenRoute)

	require.ErrorIs(t, err, fs.ErrPermission)
}

func TestGuard_Apps_ListsTopLevelEntriesBySuffix(t *testing.T) {
	g, dest := openGuard(t)
	for _, dir := range []string{"Foo.app/Contents", "Bar.APP", "docs", ".app"} {
		require.NoError(t, g.Dir(dir, 0o755))
	}
	for _, file := range []string{"notes.app", "tool.exe", "Setup.EXE", "gone.exe"} {
		require.NoError(t, g.File(context.Background(), file, 0o644, strings.NewReader("x")))
	}
	require.NoError(t, os.Remove(filepath.Join(dest, "gone.exe")))

	assert.Equal(t, []models.App{
		{Name: "Bar", Entry: filepath.Join(dest, "Bar.APP")},
		{Name: "Foo", Entry: filepath.Join(dest, "Foo.app")},
	}, g.Apps(dest, ".app", true))
	assert.Equal(t, []models.App{
		{Name: "Setup", Entry: filepath.Join(dest, "Setup.EXE")},
		{Name: "tool", Entry: filepath.Join(dest, "tool.exe")},
	}, g.Apps(dest, ".exe", false))
}

func TestWithNameRules_AppliesToEveryEntryKind(t *testing.T) {
	testCases := []struct {
		name    string
		rules   guard.NameRules
		add     func(g *guard.Guard) error
		wantErr error
	}{
		{
			name:    "reserved directory",
			rules:   guard.NameRules{WindowsNames: true},
			add:     func(g *guard.Guard) error { return g.Dir("aux", 0o755) },
			wantErr: models.ErrReservedName,
		},
		{
			name:  "reserved file",
			rules: guard.NameRules{WindowsNames: true},
			add: func(g *guard.Guard) error {
				return g.File(context.Background(), "sub/con.txt", 0o644, strings.NewReader("x"))
			},
			wantErr: models.ErrReservedName,
		},
		{
			name:    "reserved symlink",
			rules:   guard.NameRules{WindowsNames: true},
			add:     func(g *guard.Guard) error { return g.Symlink("prn", "plain") },
			wantErr: models.ErrReservedName,
		},
		{
			name:    "colliding hardlink",
			rules:   guard.NameRules{FoldCase: true},
			add:     func(g *guard.Guard) error { return g.Hardlink("PLAIN", "plain") },
			wantErr: models.ErrNameCollision,
		},
		{
			name:  "colliding file",
			rules: guard.NameRules{FoldCase: true, WindowsNames: true},
			add: func(g *guard.Guard) error {
				return g.File(context.Background(), "Plain", 0o644, strings.NewReader("y"))
			},
			wantErr: models.ErrNameCollision,
		},
		{
			name:  "ordinary names pass every rule",
			rules: guard.NameRules{FoldCase: true, WindowsNames: true},
			add: func(g *guard.Guard) error {
				return g.File(context.Background(), "plain-copy", 0o644, strings.NewReader("y"))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g, dest := openGuard(t, guard.WithNameRules(tc.rules))
			require.NoError(t, g.File(context.Background(), "plain", 0o644, strings.NewReader("x")))

			err := tc.add(g)

			assert.Equal(t, "x", mocks.ReadString(t, filepath.Join(dest, "plain")))
			if tc.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

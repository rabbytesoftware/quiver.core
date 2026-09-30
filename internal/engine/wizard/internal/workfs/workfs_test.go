package workfs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInside(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "wd")

	testCases := []struct {
		name string
		path string
		want bool
	}{
		{name: "child", path: filepath.Join(dir, "a", "b"), want: true},
		{name: "self", path: dir, want: true},
		{name: "dotted child", path: filepath.Join(dir, "..a"), want: true},
		{name: "parent", path: filepath.Dir(dir)},
		{name: "sibling", path: dir + "x"},
		{name: "escape", path: filepath.Join(dir, "..", "other")},
		{name: "relative", path: "rel"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, Inside(dir, tc.path))
		})
	}
}

func TestRelocate(t *testing.T) {
	root := t.TempDir()
	from := filepath.Join(root, "wd", "Tool.app")
	to := filepath.Join(root, "Applications", "Tool.app")

	testCases := []struct {
		name string
		path string
		want string
	}{
		{name: "inside", path: filepath.Join(from, "Contents", "tool"), want: filepath.Join(to, "Contents", "tool")},
		{name: "itself", path: from, want: to},
		{name: "outside", path: filepath.Join(root, "wd", "tool"), want: filepath.Join(root, "wd", "tool")},
		{name: "empty", path: "", want: ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, Relocate(tc.path, from, to))
		})
	}
}

func TestSwap(t *testing.T) {
	testCases := []struct {
		name      string
		previous  bool
		staged    bool
		asideBusy bool
		wantErr   bool
		wantDest  string
	}{
		{name: "replaces the previous dest", previous: true, staged: true, wantDest: "v2"},
		{name: "fills an empty dest", staged: true, wantDest: "v2"},
		{name: "a failed move into place restores the previous dest", previous: true, wantErr: true, wantDest: "v1"},
		{name: "a failed move into place with no previous dest", wantErr: true},
		{name: "a previous dest that cannot move aside stays", previous: true, staged: true, asideBusy: true, wantErr: true, wantDest: "v1"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			staged := filepath.Join(dir, "staged")
			dest := filepath.Join(dir, "dest")
			aside := filepath.Join(dir, "aside")
			if tc.previous {
				writeVersion(t, dest, "v1")
			}
			if tc.staged {
				writeVersion(t, staged, "v2")
			}
			if tc.asideBusy {
				writeVersion(t, aside, "busy")
			}

			err := Swap(staged, dest, aside, os.Rename)

			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			if tc.wantDest == "" {
				assert.NoDirExists(t, dest)
				return
			}
			data, err := os.ReadFile(filepath.Join(dest, "version"))
			require.NoError(t, err)
			assert.Equal(t, tc.wantDest, string(data))
		})
	}
}

func writeVersion(
	t *testing.T,
	dir string,
	version string,
) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "version"), []byte(version), 0o600))
}

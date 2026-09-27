package shelf

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShelf_Layout_ExplicitDirs(t *testing.T) {
	f := newFixture(t, "linux")

	l, err := f.shelf.layout()

	require.NoError(t, err)
	assert.Equal(t, layout{bin: f.bin, namespaces: f.nsDir, userHome: f.userHome, apps: f.apps}, l)
}

func TestShelf_Layout_Errors(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	nsBlocked := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(nsBlocked, "namespaces"), nil, 0o600))

	testCases := []struct {
		name string
		opts []Option
	}{
		{
			name: "bin unavailable",
			opts: []Option{WithHomeDir(file)},
		},
		{
			name: "namespaces unavailable",
			opts: []Option{WithHomeDir(nsBlocked)},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(tc.opts...).(*shelf).layout()
			require.Error(t, err)
		})
	}
}

func TestShelf_Layout_UserHomeUnavailable(t *testing.T) {
	if runtime.GOOS == goosWindows {
		t.Skip("os.UserHomeDir reads USERPROFILE on windows")
	}
	t.Setenv("HOME", "")

	_, err := New(WithHomeDir(t.TempDir())).(*shelf).layout()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "user home")
}

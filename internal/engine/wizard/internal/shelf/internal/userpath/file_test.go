package userpath

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFile_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sandbox", "user-path")
	up := NewFile(path)

	empty, err := up.Read()
	require.NoError(t, err)
	require.NoError(t, up.Write(`C:\a;C:\b`))
	got, err := up.Read()
	require.NoError(t, err)

	assert.Empty(t, empty)
	assert.Equal(t, `C:\a;C:\b`, got)
	assert.NoError(t, up.Broadcast())
	assert.Equal(t, path, up.Location())
}

func TestFile_PathIsADirectory(t *testing.T) {
	up := NewFile(t.TempDir())

	_, err := up.Read()

	assert.Error(t, err)
	assert.Error(t, up.Write("x"))
}

func TestFile_ParentIsAFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	assert.Error(t, NewFile(filepath.Join(file, "user-path")).Write("x"))
}

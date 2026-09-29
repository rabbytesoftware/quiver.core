package binary

import (
	"errors"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errReadFailed = errors.New("read failed")

type failingReaderAt struct{}

func (failingReaderAt) ReadAt(
	_ []byte,
	_ int64,
) (int, error) {
	return 0, errReadFailed
}

func TestCopyExecutable_ReadFailureRemovesOutput(t *testing.T) {
	out := filepath.Join(t.TempDir(), "tool")

	err := copyExecutable(failingReaderAt{}, 8, out)

	require.Error(t, err)
	assert.ErrorIs(t, err, errReadFailed)
	assert.NoFileExists(t, out)
}

func TestChmodExecutable_MissingFile(t *testing.T) {
	err := chmodExecutable(filepath.Join(t.TempDir(), "missing"))

	require.Error(t, err)
	assert.ErrorIs(t, err, fs.ErrNotExist)
}

package portable

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type failingReaderAt struct{}

func (failingReaderAt) ReadAt(
	_ []byte,
	_ int64,
) (int, error) {
	return 0, errors.New("read failed")
}

func TestCopyExecutable_ReadFailureRemovesOutput(t *testing.T) {
	out := filepath.Join(t.TempDir(), "tool")

	err := copyExecutable(failingReaderAt{}, 8, out)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "read failed")
	assert.NoFileExists(t, out)
}

func TestChmodExecutable_MissingFile(t *testing.T) {
	err := chmodExecutable(filepath.Join(t.TempDir(), "missing"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "portable: chmod")
}

package binary

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/mocks"
)

func detect(
	t *testing.T,
	maxBytes int64,
	path string,
) (models.Format, bool) {
	t.Helper()

	src, err := os.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = src.Close() })

	format, ok, err := New(maxBytes)(src, 0)
	require.NoError(t, err)

	return format, ok
}

func assertExecutable(
	t *testing.T,
	path string,
	want string,
) {
	t.Helper()

	assert.Equal(t, want, mocks.ReadString(t, path))
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
}

func TestNew_DetectsExecutablesOnly(t *testing.T) {
	dir := t.TempDir()

	format, ok := detect(t, mocks.TestMaxBytes, mocks.WriteFile(t, filepath.Join(dir, "tool"), []byte(mocks.ElfExecutable)))
	require.True(t, ok)
	assert.Equal(t, models.KindBinary, format.Kind())
	assert.Equal(t, models.Unit{}, format.Unit())

	_, ok = detect(t, mocks.TestMaxBytes, mocks.WriteFile(t, filepath.Join(dir, "notes"), []byte("hello")))
	assert.False(t, ok)
}

func TestUnpack_CopiesTheExecutable(t *testing.T) {
	testCases := []struct {
		name string
		as   string
		want string
	}{
		{name: "keeps the source name", want: "tool-1.2"},
		{name: "takes the target name", as: "tool", want: "tool"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			from := mocks.WriteFile(t, filepath.Join(t.TempDir(), "tool-1.2"), []byte(mocks.ElfExecutable))
			format, _ := detect(t, mocks.TestMaxBytes, from)
			to := filepath.Join(t.TempDir(), "bin")

			result, err := format.Unpack(context.Background(), models.Target{Dir: to, Name: tc.as})

			require.NoError(t, err)
			out := filepath.Join(to, tc.want)
			assert.Equal(t, models.Result{Output: out}, result)
			assertExecutable(t, out, mocks.ElfExecutable)
			assert.FileExists(t, from)
		})
	}
}

func TestUnpack_ReplacesAReadOnlyOutput(t *testing.T) {
	from := mocks.WriteFile(t, filepath.Join(t.TempDir(), "tool"), []byte(mocks.ElfExecutable))
	to := t.TempDir()
	out := mocks.WriteFile(t, filepath.Join(to, "tool"), []byte("old"))
	require.NoError(t, os.Chmod(out, 0o444))
	format, _ := detect(t, mocks.TestMaxBytes, from)

	_, err := format.Unpack(context.Background(), models.Target{Dir: to})

	require.NoError(t, err)
	assertExecutable(t, out, mocks.ElfExecutable)
}

func TestUnpack_ExecutableAlreadyInPlaceIsItsOwnOutput(t *testing.T) {
	to := t.TempDir()
	from := mocks.WriteFile(t, filepath.Join(to, "tool"), []byte(mocks.ElfExecutable))
	format, _ := detect(t, mocks.TestMaxBytes, from)

	result, err := format.Unpack(context.Background(), models.Target{Dir: to})

	require.NoError(t, err)
	assert.Equal(t, models.Result{Output: from}, result)
	assertExecutable(t, from, mocks.ElfExecutable)
}

func TestUnpack_Failures(t *testing.T) {
	testCases := []struct {
		name     string
		maxBytes int64
		target   func(t *testing.T) string
		wantErr  error
	}{
		{
			name:     "larger than the limit",
			maxBytes: 4,
			target:   func(t *testing.T) string { return t.TempDir() },
			wantErr:  models.ErrTooLarge,
		},
		{
			name:     "destination under a file",
			maxBytes: mocks.TestMaxBytes,
			target: func(t *testing.T) string {
				return filepath.Join(mocks.WriteFile(t, filepath.Join(t.TempDir(), "file"), []byte("x")), "bin")
			},
		},
		{
			name:     "output is a non-empty directory",
			maxBytes: mocks.TestMaxBytes,
			target: func(t *testing.T) string {
				to := t.TempDir()
				mocks.WriteFile(t, filepath.Join(to, "tool", "x"), []byte("x"))
				return to
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			from := mocks.WriteFile(t, filepath.Join(t.TempDir(), "tool"), []byte(mocks.ElfExecutable))
			format, _ := detect(t, tc.maxBytes, from)

			_, err := format.Unpack(context.Background(), models.Target{Dir: tc.target(t)})

			require.Error(t, err)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
			}
		})
	}
}

func TestUnpack_ClosedSourceFails(t *testing.T) {
	from := mocks.WriteFile(t, filepath.Join(t.TempDir(), "tool"), []byte(mocks.ElfExecutable))
	src, err := os.Open(from)
	require.NoError(t, err)
	format, ok, err := New(mocks.TestMaxBytes)(src, 0)
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, src.Close())

	to := t.TempDir()

	_, err = format.Unpack(context.Background(), models.Target{Dir: to})

	require.Error(t, err)
	assert.NoFileExists(t, filepath.Join(to, "tool"))
}

func TestSizeError_DescribesTheLimit(t *testing.T) {
	err := sizeError{path: "tool", size: 10, limit: 4}

	assert.NotEmpty(t, err.Error())
	assert.ErrorIs(t, err, models.ErrTooLarge)
}

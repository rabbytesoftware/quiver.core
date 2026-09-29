package dmg_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/dmg"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/guard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/mocks"
)

var errAttach = errors.New("attach failed")

func openImage(
	t *testing.T,
	data []byte,
) (*os.File, int64) {
	t.Helper()

	path := mocks.WriteFile(t, filepath.Join(t.TempDir(), "image.dmg"), data)
	src, err := os.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = src.Close() })
	info, err := src.Stat()
	require.NoError(t, err)

	return src, info.Size()
}

func detectWith(
	t *testing.T,
	detect models.Detect,
) models.Format {
	t.Helper()

	src, size := openImage(t, mocks.DmgTrailer())
	format, ok, err := detect(src, size)
	require.NoError(t, err)
	require.True(t, ok)

	return format
}

func TestNew_Detection(t *testing.T) {
	testCases := []struct {
		name string
		data []byte
		want bool
	}{
		{name: "koly trailer", data: mocks.DmgTrailer(), want: true},
		{name: "no trailer", data: make([]byte, 1024), want: false},
		{name: "too short", data: []byte("short"), want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			src, size := openImage(t, tc.data)

			format, ok, err := dmg.New(mocks.TestMaxBytes, guard.NameRules{})(src, size)

			require.NoError(t, err)
			assert.Equal(t, tc.want, ok)
			if !tc.want {
				return
			}
			assert.Equal(t, models.KindDmg, format.Kind())
			assert.Equal(t, models.Unit{}, format.Unit())
		})
	}
}

func fakeVolume(
	t *testing.T,
) func(ctx context.Context, image, mount string) error {
	t.Helper()

	return func(_ context.Context, _, mount string) error {
		versions := filepath.Join(mount, "Foo.app", "Contents", "Frameworks", "X.framework", "Versions")
		mocks.WriteFile(t, filepath.Join(versions, "A", "X"), []byte("lib"))
		require.NoError(t, os.Symlink("A", filepath.Join(versions, "Current")))
		mocks.WriteFile(t, filepath.Join(mount, "Foo.app", "Contents", "MacOS", "foo"), []byte("bin"))
		mocks.WriteFile(t, filepath.Join(mount, "Foo.app", "Contents", ".keep"), []byte("x"))
		mocks.WriteFile(t, filepath.Join(mount, "README.txt"), []byte("readme"))
		mocks.WriteFile(t, filepath.Join(mount, ".hidden"), []byte("x"))
		mocks.WriteFile(t, filepath.Join(mount, ".hiddendir", "x"), []byte("x"))
		require.NoError(t, os.Symlink("/Applications", filepath.Join(mount, "Applications")))
		require.NoError(t, os.Symlink("README.txt", filepath.Join(mount, "Readme")))
		return nil
	}
}

func TestUnpack_CopiesTheVolumeAndRecordsBundles(t *testing.T) {
	detached := ""
	detach := func(_ context.Context, mount string) {
		detached = mount
		_ = os.RemoveAll(mount)
	}
	detect := dmg.NewWithMounter(mocks.TestMaxBytes, fakeVolume(t), detach)
	to := filepath.Join(t.TempDir(), "out")

	result, err := detectWith(t, detect).Unpack(context.Background(), models.Target{Dir: to})

	require.NoError(t, err)
	assert.Equal(t, []models.App{{Name: "Foo", Entry: filepath.Join(to, "Foo.app")}}, result.Apps)
	assert.Equal(t, "bin", mocks.ReadString(t, filepath.Join(to, "Foo.app", "Contents", "MacOS", "foo")))
	assert.FileExists(t, filepath.Join(to, "Foo.app", "Contents", ".keep"))
	outVersions := filepath.Join(to, "Foo.app", "Contents", "Frameworks", "X.framework", "Versions")
	target, err := os.Readlink(filepath.Join(outVersions, "Current"))
	require.NoError(t, err)
	assert.Equal(t, "A", target)
	assert.Equal(t, "lib", mocks.ReadString(t, filepath.Join(outVersions, "Current", "X")))
	target, err = os.Readlink(filepath.Join(to, "Readme"))
	require.NoError(t, err)
	assert.Equal(t, "README.txt", target)
	for _, dropped := range []string{"Applications", ".hidden", ".hiddendir"} {
		_, err := os.Lstat(filepath.Join(to, dropped))
		assert.ErrorIs(t, err, os.ErrNotExist, dropped)
	}
	assert.NotEmpty(t, detached)
}

func TestUnpack_Failures(t *testing.T) {
	testCases := []struct {
		name    string
		attach  func(ctx context.Context, image, mount string) error
		to      func(t *testing.T) string
		wantErr error
	}{
		{
			name:    "attach fails",
			attach:  func(context.Context, string, string) error { return errAttach },
			to:      func(t *testing.T) string { return filepath.Join(t.TempDir(), "out") },
			wantErr: errAttach,
		},
		{
			name: "volume holds an escaping link",
			attach: func(_ context.Context, _, mount string) error {
				return os.Symlink("../../../etc", filepath.Join(mount, "evil"))
			},
			to:      func(t *testing.T) string { return filepath.Join(t.TempDir(), "out") },
			wantErr: models.ErrEscape,
		},
		{
			name:   "destination is a file",
			attach: func(context.Context, string, string) error { return nil },
			to: func(t *testing.T) string {
				return mocks.WriteFile(t, filepath.Join(t.TempDir(), "out"), []byte("x"))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			detect := dmg.NewWithMounter(mocks.TestMaxBytes, tc.attach, func(context.Context, string) {})

			_, err := detectWith(t, detect).Unpack(context.Background(), models.Target{Dir: tc.to(t)})

			require.Error(t, err)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
			}
		})
	}
}

func TestUnpack_VolumeWalkError(t *testing.T) {
	attach := func(_ context.Context, _, mount string) error {
		return os.Remove(mount)
	}
	detect := dmg.NewWithMounter(mocks.TestMaxBytes, attach, func(context.Context, string) {})

	_, err := detectWith(t, detect).Unpack(context.Background(), models.Target{Dir: filepath.Join(t.TempDir(), "out")})

	require.ErrorIs(t, err, os.ErrNotExist)
}

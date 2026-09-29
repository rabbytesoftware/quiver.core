package dest_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/portable/internal/dest"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/mocks"
)

const (
	ownerMarker = ".quiver-portable"
	proof       = ".quiver-run"
)

var errBuild = errors.New("build failed")

func dirNames(
	t *testing.T,
	dir string,
) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}

	return names
}

type recorder struct {
	t    *testing.T
	dirs []string
	file string
	err  error
}

func (r *recorder) build(
	dir string,
) (unpack.Result, error) {
	r.dirs = append(r.dirs, dir)
	if r.err != nil {
		mocks.WriteFile(r.t, filepath.Join(dir, "partial"), []byte("x"))
		return unpack.Result{}, r.err
	}

	out := mocks.WriteFile(r.t, filepath.Join(dir, r.file), []byte(r.file))
	return unpack.Result{
		Apps:   []unpack.App{{Name: "tool", Entry: out, Icon: filepath.Join(dir, "icon.png")}},
		Output: out,
	}, nil
}

func stage(
	t *testing.T,
	workDir string,
	from string,
	to string,
	unit unpack.Unit,
	r *recorder,
) (unpack.Result, error) {
	t.Helper()

	d, err := dest.Claim(workDir, from, to)
	require.NoError(t, err)

	return d.Stage(unit, r.build)
}

func TestClaim_UnownedDestinationsAreMergedInPlace(t *testing.T) {
	outside := t.TempDir()

	testCases := []struct {
		name  string
		setup func(t *testing.T, workDir string) (string, string)
	}{
		{
			name: "source outside the workdir",
			setup: func(t *testing.T, workDir string) (string, string) {
				return filepath.Join(outside, "tool.zip"), filepath.Join(workDir, "tool")
			},
		},
		{
			name: "destination outside the workdir",
			setup: func(t *testing.T, workDir string) (string, string) {
				return filepath.Join(workDir, "tool.zip"), filepath.Join(outside, "tool")
			},
		},
		{
			name: "destination is the workdir",
			setup: func(t *testing.T, workDir string) (string, string) {
				return filepath.Join(workDir, "tool.zip"), workDir
			},
		},
		{
			name: "source inside the destination",
			setup: func(t *testing.T, workDir string) (string, string) {
				return filepath.Join(workDir, "tool", "tool.zip"), filepath.Join(workDir, "tool")
			},
		},
		{
			name: "destination without a marker",
			setup: func(t *testing.T, workDir string) (string, string) {
				mocks.WriteFile(t, filepath.Join(workDir, "tool", "notes.txt"), []byte("mine"))
				return filepath.Join(workDir, "tool.zip"), filepath.Join(workDir, "tool")
			},
		},
		{
			name: "destination marked by another source",
			setup: func(t *testing.T, workDir string) (string, string) {
				mocks.WriteFile(t, filepath.Join(workDir, "tool", ownerMarker), []byte("other.zip\n"))
				return filepath.Join(workDir, "tool.zip"), filepath.Join(workDir, "tool")
			},
		},
		{
			name: "destination is a file",
			setup: func(t *testing.T, workDir string) (string, string) {
				return filepath.Join(workDir, "tool.zip"), mocks.WriteFile(t, filepath.Join(workDir, "tool"), []byte("x"))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			workDir := t.TempDir()
			from, to := tc.setup(t, workDir)
			var dirs []string

			d, err := dest.Claim(workDir, from, to)
			require.NoError(t, err)
			_, err = d.Stage(unpack.Unit{}, func(dir string) (unpack.Result, error) {
				dirs = append(dirs, dir)
				return unpack.Result{}, nil
			})

			require.NoError(t, err)
			assert.Equal(t, []string{to}, dirs)
		})
	}
}

func TestClaim_DestinationUnderAFileFails(t *testing.T) {
	workDir := t.TempDir()
	mocks.WriteFile(t, filepath.Join(workDir, "file"), []byte("x"))

	_, err := dest.Claim(workDir, filepath.Join(workDir, "tool.zip"), filepath.Join(workDir, "file", "tool"))

	require.Error(t, err)
}

func TestStage_OwnedDestinationIsStagedMarkedAndSwapped(t *testing.T) {
	workDir := t.TempDir()
	from := mocks.WriteFile(t, filepath.Join(workDir, "dl", "tool.zip"), []byte("zip"))
	to := filepath.Join(workDir, "tool")

	first := &recorder{t: t, file: "v1"}
	_, err := stage(t, workDir, from, to, unpack.Unit{}, first)
	require.NoError(t, err)

	second := &recorder{t: t, file: "v2"}
	result, err := stage(t, workDir, from, to, unpack.Unit{}, second)

	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join(workDir, ".tool.quiver-tmp")}, second.dirs)
	assert.ElementsMatch(t, []string{ownerMarker, "v2"}, dirNames(t, to))
	assert.Equal(t, "dl/tool.zip\n", mocks.ReadString(t, filepath.Join(to, ownerMarker)))
	assert.Equal(t, unpack.Result{
		Apps:   []unpack.App{{Name: "tool", Entry: filepath.Join(to, "v2"), Icon: filepath.Join(to, "icon.png")}},
		Output: filepath.Join(to, "v2"),
	}, result)
	assert.ElementsMatch(t, []string{"dl", "tool"}, dirNames(t, workDir))
}

func TestStage_OwnedDestinationBuildsTheUnitInsideTheStaging(t *testing.T) {
	workDir := t.TempDir()
	to := filepath.Join(workDir, "tool")
	r := &recorder{t: t, file: proof}

	result, err := stage(t, workDir, filepath.Join(workDir, "tool.AppImage"), to, unpack.Unit{Dir: "tool", Proof: proof}, r)

	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join(workDir, ".tool.quiver-tmp", "tool")}, r.dirs)
	assert.Equal(t, filepath.Join(to, "tool", proof), result.Output)
	assert.ElementsMatch(t, []string{ownerMarker, "tool"}, dirNames(t, to))
}

func TestStage_OwnedFailuresKeepThePreviousInstall(t *testing.T) {
	testCases := []struct {
		name string
		r    func(t *testing.T) *recorder
	}{
		{name: "build fails", r: func(t *testing.T) *recorder { return &recorder{t: t, err: errBuild} }},
		{name: "build leaves a directory at the owner marker", r: func(t *testing.T) *recorder {
			return &recorder{t: t, file: filepath.Join(ownerMarker, "x")}
		}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			workDir := t.TempDir()
			from := filepath.Join(workDir, "tool.zip")
			to := filepath.Join(workDir, "tool")
			_, err := stage(t, workDir, from, to, unpack.Unit{}, &recorder{t: t, file: "v1"})
			require.NoError(t, err)

			_, err = stage(t, workDir, from, to, unpack.Unit{}, tc.r(t))

			require.Error(t, err)
			assert.ElementsMatch(t, []string{ownerMarker, "v1"}, dirNames(t, to))
			assert.ElementsMatch(t, []string{"tool"}, dirNames(t, workDir))
		})
	}
}

func TestStage_OwnedLeftoversAreCleared(t *testing.T) {
	workDir := t.TempDir()
	mocks.WriteFile(t, filepath.Join(workDir, ".tool.quiver-tmp", "crashed"), []byte("x"))
	mocks.WriteFile(t, filepath.Join(workDir, "apps", ".tool.quiver-old", "stale"), []byte("x"))
	mocks.WriteFile(t, filepath.Join(workDir, "apps", "tool", ownerMarker), []byte("tool.zip\n"))

	for _, to := range []string{"tool", filepath.Join("apps", "tool")} {
		_, err := stage(t, workDir, filepath.Join(workDir, "tool.zip"), filepath.Join(workDir, to), unpack.Unit{}, &recorder{t: t, file: "v1"})
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{ownerMarker, "v1"}, dirNames(t, filepath.Join(workDir, to)))
	}
	assert.ElementsMatch(t, []string{"apps", "tool"}, dirNames(t, workDir))
	assert.ElementsMatch(t, []string{"tool"}, dirNames(t, filepath.Join(workDir, "apps")))
}

func TestStage_OwnedLeftoverThatCannotBeRemovedFails(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("directory permissions do not block removal here")
	}
	workDir := t.TempDir()
	locked := filepath.Join(workDir, ".tool.quiver-tmp", "locked")
	mocks.WriteFile(t, filepath.Join(locked, "x"), []byte("x"))
	require.NoError(t, os.Chmod(locked, 0o500))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	r := &recorder{t: t, file: "v1"}

	_, err := stage(t, workDir, filepath.Join(workDir, "tool.zip"), filepath.Join(workDir, "tool"), unpack.Unit{}, r)

	require.Error(t, err)
	assert.Empty(t, r.dirs)
}

func TestClaim_RestoresAnInstallLeftAside(t *testing.T) {
	workDir := t.TempDir()
	mocks.WriteFile(t, filepath.Join(workDir, ".tool.quiver-old", ownerMarker), []byte("tool.zip\n"))
	mocks.WriteFile(t, filepath.Join(workDir, ".tool.quiver-old", "v1"), []byte("v1"))

	_, err := dest.Claim(workDir, filepath.Join(workDir, "tool.zip"), filepath.Join(workDir, "tool"))

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{ownerMarker, "v1"}, dirNames(t, filepath.Join(workDir, "tool")))
	assert.ElementsMatch(t, []string{"tool"}, dirNames(t, workDir))
}

func TestClaim_AsideThatCannotBeRestoredFails(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("directory permissions do not block renames here")
	}
	workDir := t.TempDir()
	parent := filepath.Join(workDir, "apps")
	mocks.WriteFile(t, filepath.Join(parent, ".tool.quiver-old", "v1"), []byte("v1"))
	require.NoError(t, os.Chmod(parent, 0o500))
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })

	_, err := dest.Claim(workDir, filepath.Join(workDir, "tool.zip"), filepath.Join(parent, "tool"))

	require.Error(t, err)
}

func TestStage_UnitReplacesOnlyItsOwnDirectory(t *testing.T) {
	workDir := t.TempDir()
	to := workDir
	mocks.WriteFile(t, filepath.Join(to, "notes.txt"), []byte("mine"))
	mocks.WriteFile(t, filepath.Join(to, "tool", proof), []byte("old"))
	mocks.WriteFile(t, filepath.Join(to, "tool", "stale.so"), []byte("old"))
	mocks.WriteFile(t, filepath.Join(to, ".tool.quiver-tmp", "crashed"), []byte("x"))
	r := &recorder{t: t, file: proof}

	result, err := stage(t, workDir, filepath.Join(workDir, "tool.AppImage"), to, unpack.Unit{Dir: "tool", Proof: proof}, r)

	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join(to, ".tool.quiver-tmp")}, r.dirs)
	assert.ElementsMatch(t, []string{proof}, dirNames(t, filepath.Join(to, "tool")))
	assert.Equal(t, filepath.Join(to, "tool", proof), result.Apps[0].Entry)
	assert.Equal(t, "mine", mocks.ReadString(t, filepath.Join(to, "notes.txt")))
	assert.NoDirExists(t, filepath.Join(to, ".tool.quiver-tmp"))
}

func TestStage_UnitFailures(t *testing.T) {
	testCases := []struct {
		name    string
		setup   func(t *testing.T, to string)
		r       func(t *testing.T) *recorder
		wantErr error
	}{
		{
			name: "existing directory without the proof",
			setup: func(t *testing.T, to string) {
				mocks.WriteFile(t, filepath.Join(to, "tool", "notes.txt"), []byte("mine"))
			},
			r:       func(t *testing.T) *recorder { return &recorder{t: t, file: proof} },
			wantErr: dest.ErrUnowned,
		},
		{
			name:    "build fails",
			setup:   func(t *testing.T, to string) {},
			r:       func(t *testing.T) *recorder { return &recorder{t: t, err: errBuild} },
			wantErr: errBuild,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			to := t.TempDir()
			tc.setup(t, to)

			_, err := stage(t, to, filepath.Join(t.TempDir(), "tool.AppImage"), to, unpack.Unit{Dir: "tool", Proof: proof}, tc.r(t))

			require.ErrorIs(t, err, tc.wantErr)
			assert.NoDirExists(t, filepath.Join(to, ".tool.quiver-tmp"))
			assert.NoFileExists(t, filepath.Join(to, "tool", proof))
		})
	}
}

func TestStage_UnitLeftoverThatCannotBeRemovedFails(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("directory permissions do not block removal here")
	}
	to := t.TempDir()
	locked := filepath.Join(to, ".tool.quiver-tmp", "locked")
	mocks.WriteFile(t, filepath.Join(locked, "x"), []byte("x"))
	require.NoError(t, os.Chmod(locked, 0o500))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	r := &recorder{t: t, file: proof}

	_, err := stage(t, to, filepath.Join(t.TempDir(), "tool.AppImage"), to, unpack.Unit{Dir: "tool", Proof: proof}, r)

	require.Error(t, err)
	assert.Empty(t, r.dirs)
}

func TestStage_UnitOutsideTheDestinationFails(t *testing.T) {
	to := t.TempDir()
	r := &recorder{t: t, file: proof}

	_, err := stage(t, to, filepath.Join(t.TempDir(), "x"), to, unpack.Unit{Dir: "..", Proof: proof}, r)

	require.Error(t, err)
	assert.Empty(t, r.dirs)
}

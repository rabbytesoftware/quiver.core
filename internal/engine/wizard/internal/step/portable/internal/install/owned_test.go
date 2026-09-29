package install_test

import (
	"archive/tar"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	wizstep "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/mocks"
)

const ownerMarker = ".quiver-portable"

func linuxRequest(
	workDir string,
) wizstep.Request {
	return wizstep.Request{WorkDir: workDir, OSArch: "linux/amd64"}
}

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

func TestInstall_StepsSharingADirectoryKeepEachOthersFiles(t *testing.T) {
	for _, to := range []string{".", "bin"} {
		t.Run("to "+to, func(t *testing.T) {
			workDir := t.TempDir()
			prefix := "bin/"
			if to == "bin" {
				prefix = ""
			}

			for range 2 {
				mocks.WriteFile(t, filepath.Join(workDir, "a.zip"), mocks.ZipFiles(t, map[string]string{prefix + "a": "a"}))
				require.NoError(t, runPortable(t, linuxRequest(workDir), "a.zip", to, ""))
				mocks.WriteFile(t, filepath.Join(workDir, "b.zip"), mocks.ZipFiles(t, map[string]string{prefix + "b": "b"}))
				require.NoError(t, runPortable(t, linuxRequest(workDir), "b.zip", to, ""))
			}

			assert.Equal(t, "a", mocks.ReadString(t, filepath.Join(workDir, "bin", "a")))
			assert.Equal(t, "b", mocks.ReadString(t, filepath.Join(workDir, "bin", "b")))
		})
	}
}

func TestInstall_OwnedDestinationIsReplacedAcrossFormats(t *testing.T) {
	workDir := t.TempDir()
	to := filepath.Join(workDir, "tool")
	download := filepath.Join(workDir, "tool.download")

	mocks.WriteFile(t, download, mocks.GzipBytes(t, mocks.TarBytes(t,
		mocks.TarEntry{Name: "tool-v1/tool", Body: "v1", Mode: 0o755, Flag: tar.TypeReg},
	)))
	require.NoError(t, runPortable(t, linuxRequest(workDir), "tool.download", "tool", ""))
	assert.ElementsMatch(t, []string{ownerMarker, "tool-v1"}, dirNames(t, to))

	mocks.WriteFile(t, download, mocks.ZipFiles(t, map[string]string{"tool-v2/tool": "v2"}))
	require.NoError(t, runPortable(t, linuxRequest(workDir), "tool.download", "tool", ""))
	assert.ElementsMatch(t, []string{ownerMarker, "tool-v2"}, dirNames(t, to))
	assert.Equal(t, "v2", mocks.ReadString(t, filepath.Join(to, "tool-v2", "tool")))

	mocks.WriteAppImage(t, workDir, "tool.download", mocks.AppImageEntries("tool"))
	require.NoError(t, runPortable(t, linuxRequest(workDir), "tool.download", "tool", ""))
	assert.ElementsMatch(t, []string{ownerMarker, "tool.download.AppDir"}, dirNames(t, to))
	assert.Equal(t, []domain.PortableApp{{
		Name:  "tool",
		Entry: "tool/tool.download.AppDir/.quiver-run",
		Icon:  "tool/tool.download.AppDir/usr/share/icons/hicolor/256x256/apps/tool.png",
	}}, readRecord(t, workDir).Apps)
	assert.FileExists(t, filepath.Join(to, "tool.download.AppDir", unpack.LauncherName))
	assert.NoFileExists(t, download)
	assert.NoDirExists(t, filepath.Join(workDir, ".tool.quiver-tmp"))
}

func TestInstall_FailedUnpackKeepsThePreviousInstall(t *testing.T) {
	workDir := t.TempDir()
	mocks.WriteFile(t, filepath.Join(workDir, "tool.download"), mocks.ZipFiles(t, map[string]string{"tool-v1/tool": "v1"}))
	require.NoError(t, runPortable(t, linuxRequest(workDir), "tool.download", "tool", ""))

	mocks.WriteFile(t, filepath.Join(workDir, "tool.download"), []byte{0x1f, 0x8b, 0x00})

	err := runPortable(t, linuxRequest(workDir), "tool.download", "tool", "")

	require.Error(t, err)
	assert.Equal(t, "v1", mocks.ReadString(t, filepath.Join(workDir, "tool", "tool-v1", "tool")))
	assert.NoDirExists(t, filepath.Join(workDir, ".tool.quiver-tmp"))
}

func TestInstall_UnownedExistingDestinationIsMergedNeverDeleted(t *testing.T) {
	testCases := []struct {
		name   string
		marker string
	}{
		{name: "no marker"},
		{name: "marker of another source", marker: "other.download\n"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			workDir := t.TempDir()
			mine := mocks.WriteFile(t, filepath.Join(workDir, "tool", "notes.txt"), []byte("mine"))
			if tc.marker != "" {
				mocks.WriteFile(t, filepath.Join(workDir, "tool", ownerMarker), []byte(tc.marker))
			}

			for _, version := range []string{"v1", "v2"} {
				mocks.WriteFile(t, filepath.Join(workDir, "tool.download"), mocks.ZipFiles(t, map[string]string{"tool-" + version + "/tool": version}))
				require.NoError(t, runPortable(t, linuxRequest(workDir), "tool.download", "tool", ""))
			}

			assert.Equal(t, "mine", mocks.ReadString(t, mine))
			assert.FileExists(t, filepath.Join(workDir, "tool", "tool-v1", "tool"))
			assert.FileExists(t, filepath.Join(workDir, "tool", "tool-v2", "tool"))
		})
	}
}

func installV1(
	t *testing.T,
	workDir string,
) {
	t.Helper()

	mocks.WriteFile(t, filepath.Join(workDir, "tool.download"), mocks.ZipFiles(t, map[string]string{"tool-v1/tool": "v1"}))
	require.NoError(t, runPortable(t, linuxRequest(workDir), "tool.download", "tool", ""))
}

func requirePermissionsBlock(
	t *testing.T,
) {
	t.Helper()

	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("directory permissions do not block this operation here")
	}
}

func lockDir(
	t *testing.T,
	dir string,
) {
	t.Helper()

	mocks.WriteFile(t, filepath.Join(dir, "x"), []byte("x"))
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
}

func TestInstall_OwnedDestinationFailuresKeepThePreviousInstall(t *testing.T) {
	testCases := []struct {
		name             string
		needsPermissions bool
		setup            func(t *testing.T, workDir string)
		leftover         string
	}{
		{
			name: "archive ships a directory at the owner marker",
			setup: func(t *testing.T, workDir string) {
				mocks.WriteFile(t, filepath.Join(workDir, "tool.download"), mocks.ZipFiles(t, map[string]string{ownerMarker + "/x": "x"}))
			},
		},
		{
			name:             "leftover staging cannot be removed",
			needsPermissions: true,
			setup: func(t *testing.T, workDir string) {
				lockDir(t, filepath.Join(workDir, ".tool.quiver-tmp", "locked"))
			},
			leftover: ".tool.quiver-tmp",
		},
		{
			name:             "leftover aside cannot be removed",
			needsPermissions: true,
			setup: func(t *testing.T, workDir string) {
				lockDir(t, filepath.Join(workDir, ".tool.quiver-old", "locked"))
			},
			leftover: ".tool.quiver-old",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.needsPermissions {
				requirePermissionsBlock(t)
			}
			workDir := t.TempDir()
			installV1(t, workDir)
			mocks.WriteFile(t, filepath.Join(workDir, "tool.download"), mocks.ZipFiles(t, map[string]string{"tool-v2/tool": "v2"}))
			tc.setup(t, workDir)

			err := runPortable(t, linuxRequest(workDir), "tool.download", "tool", "")

			require.Error(t, err)
			assert.ElementsMatch(t, []string{ownerMarker, "tool-v1"}, dirNames(t, filepath.Join(workDir, "tool")))
			assert.Equal(t, "v1", mocks.ReadString(t, filepath.Join(workDir, "tool", "tool-v1", "tool")))
			for _, hidden := range []string{".tool.quiver-tmp", ".tool.quiver-old"} {
				if hidden == tc.leftover {
					assert.DirExists(t, filepath.Join(workDir, hidden))
					continue
				}
				assert.NoDirExists(t, filepath.Join(workDir, hidden))
			}
		})
	}
}

func TestInstall_DestinationUnderAFileFails(t *testing.T) {
	workDir := t.TempDir()
	mocks.WriteFile(t, filepath.Join(workDir, "tool.download"), mocks.ZipFiles(t, map[string]string{"tool": "v1"}))
	mocks.WriteFile(t, filepath.Join(workDir, "file"), []byte("x"))

	err := runPortable(t, linuxRequest(workDir), "tool.download", "file/tool", "")

	require.Error(t, err)
	assert.Equal(t, "x", mocks.ReadString(t, filepath.Join(workDir, "file")))
}

func TestInstall_PreviousInstallThatCannotBeRemovedIsClearedOnTheNextRun(t *testing.T) {
	requirePermissionsBlock(t)
	workDir := t.TempDir()
	installV1(t, workDir)
	locked := filepath.Join(workDir, "tool", "tool-v1")
	require.NoError(t, os.Chmod(locked, 0o500))

	mocks.WriteFile(t, filepath.Join(workDir, "tool.download"), mocks.ZipFiles(t, map[string]string{"tool-v2/tool": "v2"}))
	require.NoError(t, runPortable(t, linuxRequest(workDir), "tool.download", "tool", ""))
	assert.ElementsMatch(t, []string{ownerMarker, "tool-v2"}, dirNames(t, filepath.Join(workDir, "tool")))
	assert.DirExists(t, filepath.Join(workDir, ".tool.quiver-old"))

	require.NoError(t, os.Chmod(filepath.Join(workDir, ".tool.quiver-old", "tool-v1"), 0o755))
	mocks.WriteFile(t, filepath.Join(workDir, "tool.download"), mocks.ZipFiles(t, map[string]string{"tool-v3/tool": "v3"}))
	require.NoError(t, runPortable(t, linuxRequest(workDir), "tool.download", "tool", ""))
	assert.ElementsMatch(t, []string{ownerMarker, "tool-v3"}, dirNames(t, filepath.Join(workDir, "tool")))
	assert.NoDirExists(t, filepath.Join(workDir, ".tool.quiver-old"))
}

func TestInstall_OwnerMarkerNeverFollowsAShippedSymlink(t *testing.T) {
	workDir := t.TempDir()
	mocks.WriteFile(t, filepath.Join(workDir, "tool.download"), mocks.TarBytes(t,
		mocks.TarEntry{Name: "tool", Body: "binary", Mode: 0o755, Flag: tar.TypeReg},
		mocks.TarEntry{Name: ownerMarker, Link: "tool", Flag: tar.TypeSymlink},
	))

	require.NoError(t, runPortable(t, linuxRequest(workDir), "tool.download", "tool", ""))

	assert.Equal(t, "binary", mocks.ReadString(t, filepath.Join(workDir, "tool", "tool")))
	info, err := os.Lstat(filepath.Join(workDir, "tool", ownerMarker))
	require.NoError(t, err)
	assert.True(t, info.Mode().IsRegular())
	assert.Equal(t, "tool.download\n", mocks.ReadString(t, filepath.Join(workDir, "tool", ownerMarker)))
}

func TestInstall_DestinationOutsideTheWorkDirIsNeverOwned(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "app")

	for _, version := range []string{"v1", "v2"} {
		workDir := t.TempDir()
		mocks.WriteFile(t, filepath.Join(workDir, "app.zip"), mocks.ZipFiles(t, map[string]string{"app-" + version + "/app": version}))
		require.NoError(t, runPortable(t, linuxRequest(workDir), "app.zip", outside, ""))
	}

	assert.ElementsMatch(t, []string{"app-v1", "app-v2"}, dirNames(t, outside))
}

func TestInstall_OwnedDestinationSwitchesBetweenArchiveAndBinary(t *testing.T) {
	workDir := t.TempDir()
	to := filepath.Join(workDir, "tool")
	download := filepath.Join(workDir, ".tool.download")

	mocks.WriteFile(t, download, mocks.ZipFiles(t, map[string]string{"tool-v1/tool": "v1"}))
	require.NoError(t, runPortableNamed(t, linuxRequest(workDir), ".tool.download", "tool", "tool"))
	assert.ElementsMatch(t, []string{ownerMarker, "tool-v1"}, dirNames(t, to))

	mocks.WriteFile(t, download, []byte(mocks.ElfExecutable))
	require.NoError(t, runPortableNamed(t, linuxRequest(workDir), ".tool.download", "tool", "tool"))
	assert.ElementsMatch(t, []string{ownerMarker, "tool"}, dirNames(t, to))
	assertExecutable(t, filepath.Join(to, "tool"), mocks.ElfExecutable)

	mocks.WriteFile(t, download, mocks.ZipFiles(t, map[string]string{"tool-v3/tool": "v3"}))
	require.NoError(t, runPortableNamed(t, linuxRequest(workDir), ".tool.download", "tool", "tool"))
	assert.ElementsMatch(t, []string{ownerMarker, "tool-v3"}, dirNames(t, to))
	assert.NoFileExists(t, download)
	assert.ElementsMatch(t, []string{"tool"}, dirNames(t, workDir))
}

func TestInstall_PreviousInstallLeftAsideByACrashIsRestored(t *testing.T) {
	testCases := []struct {
		name    string
		archive map[string]string
		corrupt bool
		want    []string
	}{
		{name: "failed install keeps the restored copy", corrupt: true, want: []string{ownerMarker, "tool-v1"}},
		{name: "successful install replaces the restored copy", archive: map[string]string{"tool-v2/tool": "v2"}, want: []string{ownerMarker, "tool-v2"}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			workDir := t.TempDir()
			installV1(t, workDir)
			require.NoError(t, os.Rename(filepath.Join(workDir, "tool"), filepath.Join(workDir, ".tool.quiver-old")))
			download := []byte{0x1f, 0x8b, 0x00}
			if !tc.corrupt {
				download = mocks.ZipFiles(t, tc.archive)
			}
			mocks.WriteFile(t, filepath.Join(workDir, "tool.download"), download)

			err := runPortable(t, linuxRequest(workDir), "tool.download", "tool", "")

			if tc.corrupt {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.ElementsMatch(t, tc.want, dirNames(t, filepath.Join(workDir, "tool")))
			assert.NoDirExists(t, filepath.Join(workDir, ".tool.quiver-old"))
			assert.NoDirExists(t, filepath.Join(workDir, ".tool.quiver-tmp"))
		})
	}
}

func TestInstall_SingleFilePayloadTakesTheStepName(t *testing.T) {
	testCases := []struct {
		name     string
		from     string
		stepName string
		want     string
	}{
		{name: "named", from: ".tool.download", stepName: "tool", want: "tool"},
		{name: "unnamed keeps the source stem", from: "tool-linux.gz", want: "tool-linux"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			workDir := t.TempDir()
			mocks.WriteFile(t, filepath.Join(workDir, tc.from), mocks.GzipBytes(t, []byte(mocks.ElfExecutable)))

			require.NoError(t, runPortableNamed(t, linuxRequest(workDir), tc.from, "tool", tc.stepName))

			assert.ElementsMatch(t, []string{ownerMarker, tc.want}, dirNames(t, filepath.Join(workDir, "tool")))
			assert.Equal(t, mocks.ElfExecutable, mocks.ReadString(t, filepath.Join(workDir, "tool", tc.want)))
		})
	}
}

func TestInstall_AsideThatCannotBeRestoredFailsTheStep(t *testing.T) {
	requirePermissionsBlock(t)
	workDir := t.TempDir()
	aside := filepath.Join(workDir, "apps", ".tool.quiver-old")
	mocks.WriteFile(t, filepath.Join(aside, "tool-v1", "tool"), []byte("v1"))
	mocks.WriteFile(t, filepath.Join(workDir, "tool.download"), mocks.ZipFiles(t, map[string]string{"tool-v2/tool": "v2"}))
	parent := filepath.Join(workDir, "apps")
	require.NoError(t, os.Chmod(parent, 0o500))
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })

	err := runPortable(t, linuxRequest(workDir), "tool.download", "apps/tool", "")

	require.Error(t, err)
	assert.Equal(t, "v1", mocks.ReadString(t, filepath.Join(aside, "tool-v1", "tool")))
	assert.NoDirExists(t, filepath.Join(parent, "tool"))
}

func TestInstall_Failures(t *testing.T) {
	testCases := []struct {
		name    string
		setup   func(t *testing.T, workDir string) string
		wantErr error
		check   func(t *testing.T, workDir string)
	}{
		{
			name:    "missing input",
			setup:   func(t *testing.T, workDir string) string { return "absent" },
			wantErr: os.ErrNotExist,
		},
		{
			name: "unknown format",
			setup: func(t *testing.T, workDir string) string {
				mocks.WriteFile(t, filepath.Join(workDir, "notes.txt"), []byte("not a package"))
				return "notes.txt"
			},
			wantErr: unpack.ErrUnknownFormat,
			check: func(t *testing.T, workDir string) {
				assert.FileExists(t, filepath.Join(workDir, "notes.txt"))
				assert.NoDirExists(t, filepath.Join(workDir, "out"))
			},
		},
		{
			name: "record cannot be written",
			setup: func(t *testing.T, workDir string) string {
				require.NoError(t, os.MkdirAll(filepath.Join(workDir, domain.PortableRecordFile, "x"), 0o755))
				mocks.WriteFile(t, filepath.Join(workDir, "Foo.zip"), mocks.ZipFiles(t, map[string]string{"Foo.app/Contents/MacOS/foo": "bin"}))
				return "Foo.zip"
			},
			check: func(t *testing.T, workDir string) {
				assert.DirExists(t, filepath.Join(workDir, "out", "Foo.app"))
				assert.FileExists(t, filepath.Join(workDir, "Foo.zip"))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			workDir := t.TempDir()
			from := tc.setup(t, workDir)

			err := runPortable(t, linuxRequest(workDir), from, "out", "")

			require.Error(t, err)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
			}
			if tc.check != nil {
				tc.check(t, workDir)
			}
		})
	}
}

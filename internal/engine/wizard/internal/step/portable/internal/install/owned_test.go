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
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/unpacktest"
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
				unpacktest.WriteFile(t, filepath.Join(workDir, "a.zip"), unpacktest.ZipFiles(t, map[string]string{prefix + "a": "a"}))
				require.NoError(t, runPortable(t, linuxRequest(workDir), "a.zip", to, ""))
				unpacktest.WriteFile(t, filepath.Join(workDir, "b.zip"), unpacktest.ZipFiles(t, map[string]string{prefix + "b": "b"}))
				require.NoError(t, runPortable(t, linuxRequest(workDir), "b.zip", to, ""))
			}

			assert.Equal(t, "a", unpacktest.ReadString(t, filepath.Join(workDir, "bin", "a")))
			assert.Equal(t, "b", unpacktest.ReadString(t, filepath.Join(workDir, "bin", "b")))
		})
	}
}

func TestInstall_UserFileInAMergedDirectorySurvivesARerun(t *testing.T) {
	workDir := t.TempDir()
	unpacktest.WriteFile(t, filepath.Join(workDir, "app.zip"), unpacktest.ZipFiles(t, map[string]string{"data/defaults.json": "{}"}))
	require.NoError(t, runPortable(t, linuxRequest(workDir), "app.zip", ".", ""))
	userDB := unpacktest.WriteFile(t, filepath.Join(workDir, "data", "user.db"), []byte("mine"))

	unpacktest.WriteFile(t, filepath.Join(workDir, "app.zip"), unpacktest.ZipFiles(t, map[string]string{"data/defaults.json": "{}"}))
	require.NoError(t, runPortable(t, linuxRequest(workDir), "app.zip", ".", ""))

	assert.Equal(t, "mine", unpacktest.ReadString(t, userDB))
	assert.NoFileExists(t, filepath.Join(workDir, ownerMarker))
}

func TestInstall_OwnedDestinationIsReplacedAcrossFormats(t *testing.T) {
	workDir := t.TempDir()
	to := filepath.Join(workDir, "tool")
	download := filepath.Join(workDir, "tool.download")

	unpacktest.WriteFile(t, download, unpacktest.GzipBytes(t, unpacktest.TarBytes(t,
		unpacktest.TarEntry{Name: "tool-v1/tool", Body: "v1", Mode: 0o755, Flag: tar.TypeReg},
	)))
	require.NoError(t, runPortable(t, linuxRequest(workDir), "tool.download", "tool", ""))
	assert.ElementsMatch(t, []string{ownerMarker, "tool-v1"}, dirNames(t, to))

	unpacktest.WriteFile(t, download, unpacktest.ZipFiles(t, map[string]string{"tool-v2/tool": "v2"}))
	require.NoError(t, runPortable(t, linuxRequest(workDir), "tool.download", "tool", ""))
	assert.ElementsMatch(t, []string{ownerMarker, "tool-v2"}, dirNames(t, to))
	assert.Equal(t, "v2", unpacktest.ReadString(t, filepath.Join(to, "tool-v2", "tool")))

	unpacktest.WriteAppImage(t, workDir, "tool.download", unpacktest.AppImageEntries("tool"))
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
	unpacktest.WriteFile(t, filepath.Join(workDir, "tool.download"), unpacktest.ZipFiles(t, map[string]string{"tool-v1/tool": "v1"}))
	require.NoError(t, runPortable(t, linuxRequest(workDir), "tool.download", "tool", ""))

	unpacktest.WriteFile(t, filepath.Join(workDir, "tool.download"), []byte{0x1f, 0x8b, 0x00})

	err := runPortable(t, linuxRequest(workDir), "tool.download", "tool", "")

	require.Error(t, err)
	assert.Equal(t, "v1", unpacktest.ReadString(t, filepath.Join(workDir, "tool", "tool-v1", "tool")))
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
			mine := unpacktest.WriteFile(t, filepath.Join(workDir, "tool", "notes.txt"), []byte("mine"))
			if tc.marker != "" {
				unpacktest.WriteFile(t, filepath.Join(workDir, "tool", ownerMarker), []byte(tc.marker))
			}

			for _, version := range []string{"v1", "v2"} {
				unpacktest.WriteFile(t, filepath.Join(workDir, "tool.download"), unpacktest.ZipFiles(t, map[string]string{"tool-" + version + "/tool": version}))
				require.NoError(t, runPortable(t, linuxRequest(workDir), "tool.download", "tool", ""))
			}

			assert.Equal(t, "mine", unpacktest.ReadString(t, mine))
			assert.FileExists(t, filepath.Join(workDir, "tool", "tool-v1", "tool"))
			assert.FileExists(t, filepath.Join(workDir, "tool", "tool-v2", "tool"))
		})
	}
}

func TestInstall_SourceOutsideTheWorkDirMerges(t *testing.T) {
	workDir := t.TempDir()
	from := unpacktest.WriteFile(t, filepath.Join(t.TempDir(), "tool.zip"), unpacktest.ZipFiles(t, map[string]string{"tool": "v1"}))

	require.NoError(t, runPortable(t, linuxRequest(workDir), from, "tool", ""))

	assert.Equal(t, "v1", unpacktest.ReadString(t, filepath.Join(workDir, "tool", "tool")))
	assert.NoFileExists(t, filepath.Join(workDir, "tool", ownerMarker))
}

func installV1(
	t *testing.T,
	workDir string,
) {
	t.Helper()

	unpacktest.WriteFile(t, filepath.Join(workDir, "tool.download"), unpacktest.ZipFiles(t, map[string]string{"tool-v1/tool": "v1"}))
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

	unpacktest.WriteFile(t, filepath.Join(dir, "x"), []byte("x"))
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
				unpacktest.WriteFile(t, filepath.Join(workDir, "tool.download"), unpacktest.ZipFiles(t, map[string]string{ownerMarker + "/x": "x"}))
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
			unpacktest.WriteFile(t, filepath.Join(workDir, "tool.download"), unpacktest.ZipFiles(t, map[string]string{"tool-v2/tool": "v2"}))
			tc.setup(t, workDir)

			err := runPortable(t, linuxRequest(workDir), "tool.download", "tool", "")

			require.Error(t, err)
			assert.ElementsMatch(t, []string{ownerMarker, "tool-v1"}, dirNames(t, filepath.Join(workDir, "tool")))
			assert.Equal(t, "v1", unpacktest.ReadString(t, filepath.Join(workDir, "tool", "tool-v1", "tool")))
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
	unpacktest.WriteFile(t, filepath.Join(workDir, "tool.download"), unpacktest.ZipFiles(t, map[string]string{"tool": "v1"}))
	unpacktest.WriteFile(t, filepath.Join(workDir, "file"), []byte("x"))

	err := runPortable(t, linuxRequest(workDir), "tool.download", "file/tool", "")

	require.Error(t, err)
	assert.Equal(t, "x", unpacktest.ReadString(t, filepath.Join(workDir, "file")))
	assert.ElementsMatch(t, []string{"file", "tool.download"}, dirNames(t, workDir))
}

func TestInstall_PreviousInstallThatCannotBeRemovedIsClearedOnTheNextRun(t *testing.T) {
	requirePermissionsBlock(t)
	workDir := t.TempDir()
	installV1(t, workDir)
	locked := filepath.Join(workDir, "tool", "tool-v1")
	require.NoError(t, os.Chmod(locked, 0o500))

	unpacktest.WriteFile(t, filepath.Join(workDir, "tool.download"), unpacktest.ZipFiles(t, map[string]string{"tool-v2/tool": "v2"}))
	require.NoError(t, runPortable(t, linuxRequest(workDir), "tool.download", "tool", ""))
	assert.ElementsMatch(t, []string{ownerMarker, "tool-v2"}, dirNames(t, filepath.Join(workDir, "tool")))
	assert.DirExists(t, filepath.Join(workDir, ".tool.quiver-old"))

	require.NoError(t, os.Chmod(filepath.Join(workDir, ".tool.quiver-old", "tool-v1"), 0o755))
	unpacktest.WriteFile(t, filepath.Join(workDir, "tool.download"), unpacktest.ZipFiles(t, map[string]string{"tool-v3/tool": "v3"}))
	require.NoError(t, runPortable(t, linuxRequest(workDir), "tool.download", "tool", ""))
	assert.ElementsMatch(t, []string{ownerMarker, "tool-v3"}, dirNames(t, filepath.Join(workDir, "tool")))
	assert.NoDirExists(t, filepath.Join(workDir, ".tool.quiver-old"))
}

func TestInstall_OwnerMarkerNeverFollowsAShippedSymlink(t *testing.T) {
	workDir := t.TempDir()
	unpacktest.WriteFile(t, filepath.Join(workDir, "tool.download"), unpacktest.TarBytes(t,
		unpacktest.TarEntry{Name: "tool", Body: "binary", Mode: 0o755, Flag: tar.TypeReg},
		unpacktest.TarEntry{Name: ownerMarker, Link: "tool", Flag: tar.TypeSymlink},
	))

	require.NoError(t, runPortable(t, linuxRequest(workDir), "tool.download", "tool", ""))

	assert.Equal(t, "binary", unpacktest.ReadString(t, filepath.Join(workDir, "tool", "tool")))
	info, err := os.Lstat(filepath.Join(workDir, "tool", ownerMarker))
	require.NoError(t, err)
	assert.True(t, info.Mode().IsRegular())
	assert.Equal(t, "tool.download\n", unpacktest.ReadString(t, filepath.Join(workDir, "tool", ownerMarker)))
}

func TestInstall_DestinationOutsideTheWorkDirIsNeverOwned(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "app")

	for _, version := range []string{"v1", "v2"} {
		workDir := t.TempDir()
		unpacktest.WriteFile(t, filepath.Join(workDir, "app.zip"), unpacktest.ZipFiles(t, map[string]string{"app-" + version + "/app": version}))
		require.NoError(t, runPortable(t, linuxRequest(workDir), "app.zip", outside, ""))
	}

	assert.ElementsMatch(t, []string{"app-v1", "app-v2"}, dirNames(t, outside))
}

func TestInstall_OwnedDestinationSwitchesBetweenArchiveAndBinary(t *testing.T) {
	workDir := t.TempDir()
	to := filepath.Join(workDir, "tool")
	download := filepath.Join(workDir, ".tool.download")

	unpacktest.WriteFile(t, download, unpacktest.ZipFiles(t, map[string]string{"tool-v1/tool": "v1"}))
	require.NoError(t, runPortableNamed(t, linuxRequest(workDir), ".tool.download", "tool", "tool"))
	assert.ElementsMatch(t, []string{ownerMarker, "tool-v1"}, dirNames(t, to))

	unpacktest.WriteFile(t, download, []byte(unpacktest.ElfExecutable))
	require.NoError(t, runPortableNamed(t, linuxRequest(workDir), ".tool.download", "tool", "tool"))
	assert.ElementsMatch(t, []string{ownerMarker, "tool"}, dirNames(t, to))
	assertExecutable(t, filepath.Join(to, "tool"), unpacktest.ElfExecutable)

	unpacktest.WriteFile(t, download, unpacktest.ZipFiles(t, map[string]string{"tool-v3/tool": "v3"}))
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
				download = unpacktest.ZipFiles(t, tc.archive)
			}
			unpacktest.WriteFile(t, filepath.Join(workDir, "tool.download"), download)

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

func TestInstall_AsideFileIsNotRestored(t *testing.T) {
	workDir := t.TempDir()
	unpacktest.WriteFile(t, filepath.Join(workDir, ".tool.quiver-old"), []byte("not a directory"))
	unpacktest.WriteFile(t, filepath.Join(workDir, "tool.download"), unpacktest.ZipFiles(t, map[string]string{"tool-v1/tool": "v1"}))

	require.NoError(t, runPortable(t, linuxRequest(workDir), "tool.download", "tool", ""))

	assert.ElementsMatch(t, []string{ownerMarker, "tool-v1"}, dirNames(t, filepath.Join(workDir, "tool")))
	assert.NoFileExists(t, filepath.Join(workDir, ".tool.quiver-old"))
}

func TestInstall_SingleFilePayloadTakesTheStepName(t *testing.T) {
	testCases := []struct {
		name     string
		from     string
		stepName string
		want     string
	}{
		{name: "named", from: ".tool.download", stepName: "tool", want: "tool"},
		{name: "windows name", from: ".tool.download", stepName: "tool.exe", want: "tool.exe"},
		{name: "unnamed keeps the source stem", from: "tool-linux.gz", want: "tool-linux"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			workDir := t.TempDir()
			unpacktest.WriteFile(t, filepath.Join(workDir, tc.from), unpacktest.GzipBytes(t, []byte(unpacktest.ElfExecutable)))

			require.NoError(t, runPortableNamed(t, linuxRequest(workDir), tc.from, "tool", tc.stepName))

			assert.ElementsMatch(t, []string{ownerMarker, tc.want}, dirNames(t, filepath.Join(workDir, "tool")))
			assert.Equal(t, unpacktest.ElfExecutable, unpacktest.ReadString(t, filepath.Join(workDir, "tool", tc.want)))
		})
	}
}

func TestInstall_AsideThatCannotBeRestoredFailsTheStep(t *testing.T) {
	requirePermissionsBlock(t)
	workDir := t.TempDir()
	aside := filepath.Join(workDir, "apps", ".tool.quiver-old")
	unpacktest.WriteFile(t, filepath.Join(aside, "tool-v1", "tool"), []byte("v1"))
	unpacktest.WriteFile(t, filepath.Join(workDir, "tool.download"), unpacktest.ZipFiles(t, map[string]string{"tool-v2/tool": "v2"}))
	parent := filepath.Join(workDir, "apps")
	require.NoError(t, os.Chmod(parent, 0o500))
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })

	err := runPortable(t, linuxRequest(workDir), "tool.download", "apps/tool", "")

	require.Error(t, err)
	assert.Equal(t, "v1", unpacktest.ReadString(t, filepath.Join(aside, "tool-v1", "tool")))
	assert.NoDirExists(t, filepath.Join(parent, "tool"))
}

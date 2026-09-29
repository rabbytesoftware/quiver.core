package install_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	wizstep "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/portable/internal/install"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/unpacktest"
)

func assertExecutable(
	t *testing.T,
	path string,
	want string,
) {
	t.Helper()

	assert.Equal(t, want, unpacktest.ReadString(t, path))
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
}

func TestInstall_Executable(t *testing.T) {
	testCases := []struct {
		name  string
		magic string
	}{
		{name: "elf", magic: "\x7fELF"},
		{name: "mach-o big endian", magic: "\xfe\xed\xfa\xcf"},
		{name: "mach-o little endian", magic: "\xcf\xfa\xed\xfe"},
		{name: "mach-o universal", magic: "\xca\xfe\xba\xbe"},
		{name: "pe", magic: "MZ"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			workDir := t.TempDir()
			body := tc.magic + "payload"
			from := unpacktest.WriteFile(t, filepath.Join(workDir, "dl", "tool"), []byte(body))

			err := runPortable(t, wizstep.Request{WorkDir: workDir}, "dl/tool", "bin", "")

			require.NoError(t, err)
			assertExecutable(t, filepath.Join(workDir, "bin", "tool"), body)
			assert.NoFileExists(t, from)
			assert.NoFileExists(t, filepath.Join(workDir, domain.PortableRecordFile))
		})
	}
}

func TestInstall_ExecutableReplacesExistingOutput(t *testing.T) {
	workDir := t.TempDir()
	unpacktest.WriteFile(t, filepath.Join(workDir, "dl", "tool"), []byte(unpacktest.ElfExecutable))
	out := unpacktest.WriteFile(t, filepath.Join(workDir, "bin", "tool"), []byte("old"))
	require.NoError(t, os.Chmod(out, 0o444))

	err := runPortable(t, wizstep.Request{WorkDir: workDir}, "dl/tool", "bin", "")

	require.NoError(t, err)
	assertExecutable(t, out, unpacktest.ElfExecutable)
}

func TestInstall_ExecutableAlreadyInPlaceIsKept(t *testing.T) {
	workDir := t.TempDir()
	from := unpacktest.WriteFile(t, filepath.Join(workDir, "bin", "tool"), []byte(unpacktest.ElfExecutable))

	err := runPortable(t, wizstep.Request{WorkDir: workDir}, "bin/tool", "bin", "")

	require.NoError(t, err)
	assertExecutable(t, from, unpacktest.ElfExecutable)
}

func TestInstall_ExecutableFailures(t *testing.T) {
	testCases := []struct {
		name     string
		maxBytes int64
		setup    func(t *testing.T, workDir string)
		wantErr  error
		check    func(t *testing.T, workDir string)
	}{
		{
			name:     "larger than the limit",
			maxBytes: 4,
			setup:    func(t *testing.T, workDir string) {},
			wantErr:  unpack.ErrTooLarge,
		},
		{
			name:     "destination is a file",
			maxBytes: unpacktest.TestMaxBytes,
			setup: func(t *testing.T, workDir string) {
				unpacktest.WriteFile(t, filepath.Join(workDir, "bin"), []byte("x"))
			},
			check: func(t *testing.T, workDir string) {
				assert.Equal(t, "x", unpacktest.ReadString(t, filepath.Join(workDir, "bin")))
			},
		},
		{
			name:     "output is a non-empty directory",
			maxBytes: unpacktest.TestMaxBytes,
			setup: func(t *testing.T, workDir string) {
				unpacktest.WriteFile(t, filepath.Join(workDir, "bin", "tool", "x"), []byte("x"))
			},
			wantErr: fs.ErrExist,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			workDir := t.TempDir()
			from := unpacktest.WriteFile(t, filepath.Join(workDir, "dl", "tool"), []byte(unpacktest.ElfExecutable))
			tc.setup(t, workDir)

			err := runPortableWith(t, tc.maxBytes, wizstep.Request{WorkDir: workDir}, "dl/tool", "bin", "", "")

			require.Error(t, err)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
			}
			if tc.check != nil {
				tc.check(t, workDir)
			}
			assert.FileExists(t, from)
		})
	}
}

func TestInstall_ExecutableName(t *testing.T) {
	testCases := []struct {
		name     string
		from     string
		stepName string
		body     string
		want     string
	}{
		{name: "default keeps the source file name", from: "tool-1.2", body: unpacktest.ElfExecutable, want: "tool-1.2"},
		{name: "explicit name", from: ".tool.download", stepName: "tool", body: unpacktest.ElfExecutable, want: "tool"},
		{name: "windows executable named by the step", from: ".tool.download", stepName: "tool.exe", body: "MZpayload", want: "tool.exe"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			workDir := t.TempDir()
			from := unpacktest.WriteFile(t, filepath.Join(workDir, "dl", tc.from), []byte(tc.body))

			require.NoError(t, runPortableNamed(t, wizstep.Request{WorkDir: workDir}, "dl/"+tc.from, "bin", tc.stepName))

			assertExecutable(t, filepath.Join(workDir, "bin", tc.want), tc.body)
			assert.ElementsMatch(t, []string{ownerMarker, tc.want}, dirNames(t, filepath.Join(workDir, "bin")))
			assert.NoFileExists(t, from)
		})
	}
}

func TestInstall_ExecutableUnsafeNameFails(t *testing.T) {
	for _, name := range []string{"a/b", "..", "../tool", "."} {
		t.Run(name, func(t *testing.T) {
			workDir := t.TempDir()
			from := unpacktest.WriteFile(t, filepath.Join(workDir, "dl", "tool"), []byte(unpacktest.ElfExecutable))

			err := runPortableNamed(t, wizstep.Request{WorkDir: workDir}, "dl/tool", "bin", name)

			require.ErrorIs(t, err, install.ErrInvalidName)
			assert.FileExists(t, from)
			assert.NoDirExists(t, filepath.Join(workDir, "bin"))
		})
	}
}

func TestInstall_NameIgnoredForArchives(t *testing.T) {
	workDir := t.TempDir()
	unpacktest.WriteFile(t, filepath.Join(workDir, "tool.zip"), unpacktest.ZipFiles(t, map[string]string{"tool-v1/tool": "v1"}))

	require.NoError(t, runPortableNamed(t, wizstep.Request{WorkDir: workDir}, "tool.zip", "bin", "renamed"))

	assert.ElementsMatch(t, []string{ownerMarker, "tool-v1"}, dirNames(t, filepath.Join(workDir, "bin")))
}

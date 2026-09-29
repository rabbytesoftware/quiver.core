package install_test

import (
	"archive/tar"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	wizstep "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/mocks"
)

func TestInstall_ArchiveWithoutBundleRecordsNothing(t *testing.T) {
	workDir := t.TempDir()
	from := mocks.WriteFile(t, filepath.Join(workDir, "tool.zip"), mocks.ZipFiles(t, map[string]string{
		"tool/bin/tool": "bin",
	}))

	err := runPortable(t, wizstep.Request{WorkDir: workDir}, "tool.zip", ".", "")

	require.NoError(t, err)
	assert.Equal(t, "bin", mocks.ReadString(t, filepath.Join(workDir, "tool", "bin", "tool")))
	assert.NoFileExists(t, filepath.Join(workDir, domain.PortableRecordFile))
	assert.NoFileExists(t, from)
}

func TestInstall_ArchiveRecordsTopLevelBundles(t *testing.T) {
	workDir := t.TempDir()
	from := mocks.WriteFile(t, filepath.Join(workDir, "dl", "Foo.zip"), mocks.ZipFiles(t, map[string]string{
		"Foo.app/Contents/MacOS/foo": "bin",
		"README.txt":                 "readme",
	}))

	err := runPortable(t, wizstep.Request{WorkDir: workDir}, "dl/Foo.zip", "apps", "")

	require.NoError(t, err)
	assert.Equal(t, domain.PortableRecord{Apps: []domain.PortableApp{{
		Name:  "Foo",
		Entry: "apps/Foo.app",
	}}}, readRecord(t, workDir))
	assert.NoFileExists(t, from)
}

func TestInstall_ArchiveFileNamedLikeABundleRecordsNothing(t *testing.T) {
	workDir := t.TempDir()
	mocks.WriteFile(t, filepath.Join(workDir, "a.tar"), mocks.TarBytes(t,
		mocks.TarEntry{Name: "notes.app", Body: "not a bundle", Mode: 0o644, Flag: tar.TypeReg},
		mocks.TarEntry{Name: "bin/tool", Body: "tool", Mode: 0o755, Flag: tar.TypeReg},
	))

	err := runPortable(t, wizstep.Request{WorkDir: workDir}, "a.tar", ".", "")

	require.NoError(t, err)
	assert.Equal(t, "not a bundle", mocks.ReadString(t, filepath.Join(workDir, "notes.app")))
	assert.NoFileExists(t, filepath.Join(workDir, domain.PortableRecordFile))
}

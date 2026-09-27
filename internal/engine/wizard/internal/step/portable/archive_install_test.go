package portable_test

import (
	"archive/tar"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	wizstep "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/unpack/unpacktest"
)

func TestHandler_Execute_ArchiveOnLinuxRecordsNothing(t *testing.T) {
	workDir := t.TempDir()
	from := writeFile(t, filepath.Join(workDir, "Foo.zip"), zipBytes(t, map[string]string{
		"Foo.app/Contents/MacOS/foo": "bin",
	}))

	err := runPortable(t, wizstep.Request{WorkDir: workDir, OSArch: "linux/amd64"}, "Foo.zip", ".", "")

	require.NoError(t, err)
	assert.Equal(t, "bin", unpacktest.ReadString(t, filepath.Join(workDir, "Foo.app", "Contents", "MacOS", "foo")))
	assert.NoFileExists(t, filepath.Join(workDir, domain.PortableRecordFile))
	assert.NoFileExists(t, from)
}

func TestHandler_Execute_ArchiveOnDarwinRecordsApp(t *testing.T) {
	workDir := t.TempDir()
	from := writeFile(t, filepath.Join(workDir, "dl", "Foo.zip"), zipBytes(t, map[string]string{
		"Foo.app/Contents/MacOS/foo": "bin",
		"README.txt":                 "readme",
	}))

	err := runPortable(t, wizstep.Request{WorkDir: workDir, OSArch: "darwin/arm64"}, "dl/Foo.zip", "apps", "")

	require.NoError(t, err)
	assert.Equal(t, domain.PortableRecord{Apps: []domain.PortableApp{{
		Name:  "Foo",
		Entry: "apps/Foo.app",
	}}}, readRecord(t, workDir))
	assert.NoFileExists(t, from)
}

func TestHandler_Execute_ArchiveOnDarwinWithoutBundleRecordsNothing(t *testing.T) {
	workDir := t.TempDir()
	writeFile(t, filepath.Join(workDir, "a.tar"), unpacktest.TarBytes(t,
		unpacktest.TarEntry{Name: "notes.app", Body: "not a bundle", Mode: 0o644, Flag: tar.TypeReg},
		unpacktest.TarEntry{Name: "bin/tool", Body: "tool", Mode: 0o755, Flag: tar.TypeReg},
	))

	err := runPortable(t, wizstep.Request{WorkDir: workDir, OSArch: "darwin/amd64"}, "a.tar", ".", "")

	require.NoError(t, err)
	assert.Equal(t, "not a bundle", unpacktest.ReadString(t, filepath.Join(workDir, "notes.app")))
	assert.NoFileExists(t, filepath.Join(workDir, domain.PortableRecordFile))
}

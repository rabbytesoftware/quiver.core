package install_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/portable/internal/install"
)

func recordApp(
	t *testing.T,
	workDir string,
	name string,
) domain.PortableApp {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Join(workDir, name), 0o755))

	return domain.PortableApp{
		Name:  name,
		Entry: filepath.Join(workDir, name, ".quiver-run"),
		Icon:  filepath.Join(workDir, name, "icon.png"),
	}
}

func relativeApp(
	name string,
) domain.PortableApp {
	return domain.PortableApp{Name: name, Entry: name + "/.quiver-run", Icon: name + "/icon.png"}
}

func record(
	t *testing.T,
	workDir string,
	apps ...domain.PortableApp,
) error {
	t.Helper()

	return install.New(1).Record(context.Background(), "ns", workDir, apps)
}

func TestInstaller_Record_RerunReplacesEntryAndKeepsOthers(t *testing.T) {
	workDir := t.TempDir()
	for _, name := range []string{"bruno", "bruno", "zed"} {
		require.NoError(t, record(t, workDir, recordApp(t, workDir, name)))
	}

	assert.Equal(t, domain.PortableRecord{Apps: []domain.PortableApp{relativeApp("bruno"), relativeApp("zed")}}, readRecord(t, workDir))
}

func TestInstaller_Record_MalformedRecordIsReplaced(t *testing.T) {
	workDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workDir, domain.PortableRecordFile), []byte("{not json"), 0o600))

	require.NoError(t, record(t, workDir, recordApp(t, workDir, "bruno")))

	assert.Equal(t, domain.PortableRecord{Apps: []domain.PortableApp{relativeApp("bruno")}}, readRecord(t, workDir))
}

func TestInstaller_Record_UnreadableRecordFails(t *testing.T) {
	workDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, domain.PortableRecordFile, "x"), 0o755))

	assert.Error(t, record(t, workDir, recordApp(t, workDir, "bruno")))
}

func TestInstaller_Record_NothingToRecordWritesNothing(t *testing.T) {
	workDir := t.TempDir()

	require.NoError(t, record(t, workDir))
	require.NoError(t, record(t, workDir, recordApp(t, t.TempDir(), "bruno")))

	assert.NoFileExists(t, filepath.Join(workDir, domain.PortableRecordFile))
}

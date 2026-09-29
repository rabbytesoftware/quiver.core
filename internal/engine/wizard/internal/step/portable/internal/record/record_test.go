package record_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/portable/internal/record"
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

func save(
	t *testing.T,
	workDir string,
	apps ...domain.PortableApp,
) error {
	t.Helper()

	return record.Save(context.Background(), "ns", workDir, apps)
}

func TestSave_RerunReplacesEntryAndKeepsOthers(t *testing.T) {
	workDir := t.TempDir()
	for _, name := range []string{"bruno", "bruno", "zed"} {
		require.NoError(t, save(t, workDir, recordApp(t, workDir, name)))
	}

	assert.Equal(t, domain.PortableRecord{Apps: []domain.PortableApp{relativeApp("bruno"), relativeApp("zed")}}, readRecord(t, workDir))
}

func TestSave_MalformedRecordIsReplaced(t *testing.T) {
	workDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workDir, domain.PortableRecordFile), []byte("{not json"), 0o600))

	require.NoError(t, save(t, workDir, recordApp(t, workDir, "bruno")))

	assert.Equal(t, domain.PortableRecord{Apps: []domain.PortableApp{relativeApp("bruno")}}, readRecord(t, workDir))
}

func TestSave_UnreadableRecordFails(t *testing.T) {
	workDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, domain.PortableRecordFile, "x"), 0o755))

	assert.Error(t, save(t, workDir, recordApp(t, workDir, "bruno")))
}

func TestSave_NothingToRecordWritesNothing(t *testing.T) {
	workDir := t.TempDir()

	require.NoError(t, save(t, workDir))
	require.NoError(t, save(t, workDir, recordApp(t, t.TempDir(), "bruno")))

	assert.NoFileExists(t, filepath.Join(workDir, domain.PortableRecordFile))
}

func readRecord(
	t *testing.T,
	workDir string,
) domain.PortableRecord {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(workDir, domain.PortableRecordFile))
	require.NoError(t, err)

	var got domain.PortableRecord
	require.NoError(t, json.Unmarshal(data, &got))

	return got
}

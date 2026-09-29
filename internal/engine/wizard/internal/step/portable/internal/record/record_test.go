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

func app(
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

func TestApps_RecordsAppsRelativeToWorkDir(t *testing.T) {
	workDir := t.TempDir()

	require.NoError(t, record.Apps(context.Background(), "ns", workDir, []domain.PortableApp{app(t, workDir, "bruno")}))

	assert.Equal(t, domain.PortableRecord{Apps: []domain.PortableApp{relativeApp("bruno")}}, readRecord(t, workDir))
}

func TestApps_RerunReplacesEntryAndKeepsOthers(t *testing.T) {
	workDir := t.TempDir()
	for _, name := range []string{"bruno", "bruno", "zed"} {
		require.NoError(t, record.Apps(context.Background(), "ns", workDir, []domain.PortableApp{app(t, workDir, name)}))
	}

	assert.Equal(t, domain.PortableRecord{Apps: []domain.PortableApp{relativeApp("bruno"), relativeApp("zed")}}, readRecord(t, workDir))
}

func TestApps_MalformedRecordIsReplaced(t *testing.T) {
	workDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workDir, domain.PortableRecordFile), []byte("{not json"), 0o600))

	require.NoError(t, record.Apps(context.Background(), "ns", workDir, []domain.PortableApp{app(t, workDir, "bruno")}))

	assert.Equal(t, domain.PortableRecord{Apps: []domain.PortableApp{relativeApp("bruno")}}, readRecord(t, workDir))
}

func TestApps_UnreadableRecordFails(t *testing.T) {
	workDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, domain.PortableRecordFile, "x"), 0o755))

	err := record.Apps(context.Background(), "ns", workDir, []domain.PortableApp{app(t, workDir, "bruno")})

	assert.Error(t, err)
}

func TestApps_AppOutsideWorkDirRecordsNothing(t *testing.T) {
	workDir := t.TempDir()

	err := record.Apps(context.Background(), "ns", workDir, []domain.PortableApp{app(t, t.TempDir(), "bruno")})

	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(workDir, domain.PortableRecordFile))
}

package portable

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestWriteRecord_TargetIsNonEmptyDirectory(t *testing.T) {
	workDir := t.TempDir()
	path := filepath.Join(workDir, domain.PortableRecordFile)
	require.NoError(t, os.MkdirAll(filepath.Join(path, "x"), 0o755))

	err := writeRecord(path, domain.PortableRecord{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "portable: write record")
	entries, err := os.ReadDir(workDir)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

func TestWriteRecord_MissingDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", domain.PortableRecordFile)

	err := writeRecord(path, domain.PortableRecord{})

	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestWriteRecord_ReplacesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), domain.PortableRecordFile)
	require.NoError(t, os.WriteFile(path, []byte("old"), 0o600))
	want := domain.PortableRecord{Apps: []domain.PortableApp{{Name: "A", Entry: "a"}}}

	require.NoError(t, writeRecord(path, want))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var got domain.PortableRecord
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, want, got)
}

func TestCommitRecord_ClosedFile(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "record-*")
	require.NoError(t, err)
	require.NoError(t, tmp.Close())

	err = commitRecord(tmp, domain.PortableRecord{}, filepath.Join(t.TempDir(), "out"))

	require.Error(t, err)
}

func TestRecordApps_EmptyAppsWritesNothing(t *testing.T) {
	workDir := t.TempDir()

	require.NoError(t, recordApps(context.Background(), "ns", workDir, nil))

	assert.NoFileExists(t, filepath.Join(workDir, domain.PortableRecordFile))
}

func TestRelativeApps_OutsidePaths(t *testing.T) {
	workDir := t.TempDir()
	outside := t.TempDir()

	testCases := []struct {
		name        string
		app         domain.PortableApp
		wantOutside string
	}{
		{
			name:        "entry outside",
			app:         domain.PortableApp{Name: "A", Entry: filepath.Join(outside, "a")},
			wantOutside: filepath.Join(outside, "a"),
		},
		{
			name:        "icon outside",
			app:         domain.PortableApp{Name: "A", Entry: filepath.Join(workDir, "a"), Icon: filepath.Join(outside, "a.png")},
			wantOutside: filepath.Join(outside, "a.png"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			apps, outside := relativeApps(workDir, []domain.PortableApp{tc.app})

			assert.Equal(t, tc.wantOutside, outside)
			assert.Nil(t, apps)
		})
	}
}

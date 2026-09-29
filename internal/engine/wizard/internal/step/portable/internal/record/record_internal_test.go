package record

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestWriteRecord_Failures(t *testing.T) {
	blocked := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(blocked, domain.PortableRecordFile, "x"), 0o755))

	testCases := []struct {
		name string
		path string
	}{
		{name: "target is a non-empty directory", path: filepath.Join(blocked, domain.PortableRecordFile)},
		{name: "missing directory", path: filepath.Join(t.TempDir(), "missing", domain.PortableRecordFile)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Error(t, writeRecord(tc.path, domain.PortableRecord{}))
		})
	}

	entries, err := os.ReadDir(blocked)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

func TestCommitRecord_ClosedFile(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "record-*")
	require.NoError(t, err)
	require.NoError(t, tmp.Close())

	require.Error(t, commitRecord(tmp, domain.PortableRecord{}, filepath.Join(t.TempDir(), "out")))
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
			apps, got := relativeApps(workDir, []domain.PortableApp{tc.app})

			assert.Equal(t, tc.wantOutside, got)
			assert.Nil(t, apps)
		})
	}
}

package install

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

func TestWorkdirRel(t *testing.T) {
	workDir := t.TempDir()
	nested := filepath.Join(workDir, "a", "b")
	require.NoError(t, os.MkdirAll(nested, 0o755))
	outside := t.TempDir()
	inside := filepath.Join(workDir, "file")
	require.NoError(t, os.WriteFile(inside, []byte("x"), 0o600))
	outsideLink := filepath.Join(outside, "link")
	require.NoError(t, os.Symlink(inside, outsideLink))
	escapingDir := filepath.Join(workDir, "escape")
	require.NoError(t, os.Symlink(outside, escapingDir))
	insideLink := filepath.Join(workDir, "inside-link")
	require.NoError(t, os.Symlink(outsideLink, insideLink))
	linkedWorkDir := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(workDir, linkedWorkDir))

	testCases := []struct {
		name    string
		workDir string
		path    string
		want    string
		wantOK  bool
	}{
		{name: "nested path", workDir: workDir, path: nested, want: "a/b", wantOK: true},
		{name: "missing nested path", workDir: workDir, path: filepath.Join(workDir, "gone"), want: "gone", wantOK: true},
		{name: "outside path", workDir: workDir, path: outside},
		{name: "missing outside parent", workDir: workDir, path: filepath.Join(outside, "gone", "x")},
		{name: "workdir itself", workDir: workDir, path: workDir},
		{name: "empty workdir", workDir: "", path: nested},
		{name: "outside link to an inside file", workDir: workDir, path: outsideLink},
		{name: "path through an inside link to an outside directory", workDir: workDir, path: filepath.Join(escapingDir, "link")},
		{name: "inside link to an outside file", workDir: workDir, path: insideLink, want: "inside-link", wantOK: true},
		{name: "symlinked workdir", workDir: linkedWorkDir, path: inside, want: "file", wantOK: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := workdirRel(tc.workDir, tc.path)

			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

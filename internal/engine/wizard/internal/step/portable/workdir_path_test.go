package portable

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkDirRel_Paths(t *testing.T) {
	workDir := t.TempDir()
	nested := filepath.Join(workDir, "a", "b")
	require.NoError(t, os.MkdirAll(nested, 0o755))
	outside := t.TempDir()

	testCases := []struct {
		name    string
		workDir string
		path    string
		want    string
		wantOK  bool
	}{
		{name: "nested path", workDir: workDir, path: nested, want: "a/b", wantOK: true},
		{name: "missing nested path", workDir: workDir, path: filepath.Join(workDir, "gone"), want: "gone", wantOK: true},
		{name: "outside path", workDir: workDir, path: outside, wantOK: false},
		{name: "missing outside path", workDir: workDir, path: filepath.Join(outside, "gone"), wantOK: false},
		{name: "missing outside parent", workDir: workDir, path: filepath.Join(outside, "gone", "x"), wantOK: false},
		{name: "workdir itself", workDir: workDir, path: workDir, wantOK: false},
		{name: "empty workdir", workDir: "", path: nested, wantOK: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := workDirRel(tc.workDir, tc.path)

			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestWorkDirRel_SymlinksCrossingTheWorkDir(t *testing.T) {
	workDir := t.TempDir()
	outside := t.TempDir()
	inside := filepath.Join(workDir, "file")
	require.NoError(t, os.WriteFile(inside, []byte("x"), 0o600))
	outsideLink := filepath.Join(outside, "link")
	require.NoError(t, os.Symlink(inside, outsideLink))
	escapingDir := filepath.Join(workDir, "escape")
	require.NoError(t, os.Symlink(outside, escapingDir))
	insideLink := filepath.Join(workDir, "inside-link")
	require.NoError(t, os.Symlink(outsideLink, insideLink))

	testCases := []struct {
		name   string
		path   string
		want   string
		wantOK bool
	}{
		{name: "outside link to an inside file", path: outsideLink, wantOK: false},
		{name: "path through an inside link to an outside directory", path: filepath.Join(escapingDir, "link"), wantOK: false},
		{name: "inside link to an outside file", path: insideLink, want: "inside-link", wantOK: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := workDirRel(workDir, tc.path)

			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestWorkDirRel_ResolvesSymlinkedWorkDir(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(real, link))
	file := filepath.Join(real, "app")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))

	got, ok := workDirRel(link, file)

	assert.True(t, ok)
	assert.Equal(t, "app", got)
}

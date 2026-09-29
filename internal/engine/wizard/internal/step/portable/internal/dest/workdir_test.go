package dest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
			got, ok := WorkdirRel(tc.workDir, tc.path)

			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

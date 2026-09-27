package shelf

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestWorkdirOwner(t *testing.T) {
	nsDir := filepath.Join(t.TempDir(), "namespaces")

	testCases := []struct {
		name   string
		target string
		want   domain.Namespace
	}{
		{
			name:   "versioned workdir",
			target: filepath.Join(nsDir, "github.com", "u", "r@v1", "bin", "rg"),
			want:   "github.com/u/r",
		},
		{
			name:   "workdir itself",
			target: filepath.Join(nsDir, "github.com", "u", "r@v1"),
			want:   "github.com/u/r",
		},
		{
			name:   "quiver hosted",
			target: filepath.Join(nsDir, "q.io", "u", "r", "auid@v2", "x"),
			want:   "q.io/u/r/auid",
		},
		{
			name:   "ref with slash",
			target: filepath.Join(nsDir, "github.com", "u", "r@feature", "x", "rg"),
			want:   "github.com/u/r",
		},
		{
			name:   "no ref",
			target: filepath.Join(nsDir, "github.com", "u", "r", "rg"),
			want:   "",
		},
		{
			name:   "outside",
			target: filepath.Join(filepath.Dir(nsDir), "elsewhere@v1", "rg"),
			want:   "",
		},
		{
			name:   "namespaces dir",
			target: nsDir,
			want:   "",
		},
		{
			name:   "relative",
			target: "relative@v1/rg",
			want:   "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, workdirOwner(nsDir, tc.target))
		})
	}
}

func TestMarkerOwner(t *testing.T) {
	testCases := []struct {
		name    string
		content string
		want    domain.Namespace
		wantOK  bool
	}{
		{name: "crlf", content: "@echo off\r\nREM quiver:github.com/u/r\r\n", want: "github.com/u/r", wantOK: true},
		{name: "lf", content: "a\nREM quiver: github.com/u/r \nb", want: "github.com/u/r", wantOK: true},
		{name: "absent", content: "@echo off\r\n", want: "", wantOK: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := markerOwner(tc.content, "REM quiver:")
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.wantOK, ok)
		})
	}
}

func TestFileHolder(t *testing.T) {
	dir := t.TempDir()
	marked := filepath.Join(dir, "marked")
	require.NoError(t, os.WriteFile(marked, []byte("K=github.com/u/r\n"), 0o600))
	plain := filepath.Join(dir, "plain")
	require.NoError(t, os.WriteFile(plain, []byte("hello"), 0o600))

	testCases := []struct {
		name string
		path string
		want holder
	}{
		{name: "absent", path: filepath.Join(dir, "missing"), want: holder{}},
		{name: "directory", path: dir, want: holder{exists: true}},
		{name: "marked", path: marked, want: holder{exists: true, namespace: "github.com/u/r"}},
		{name: "plain", path: plain, want: holder{exists: true}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := fileHolder(tc.path, "K=")
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestFileHolder_Errors(t *testing.T) {
	requireUnixHost(t)
	if os.Geteuid() == 0 {
		t.Skip("root reads unreadable files")
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	unreadable := filepath.Join(dir, "unreadable")
	require.NoError(t, os.WriteFile(unreadable, []byte("x"), 0o600))
	require.NoError(t, os.Chmod(unreadable, 0o000))

	testCases := []struct {
		name string
		path string
	}{
		{name: "lstat fails", path: filepath.Join(file, "child")},
		{name: "read fails", path: unreadable},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := fileHolder(tc.path, "K=")
			require.Error(t, err)
		})
	}
}

package shelf

import (
	"errors"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubDirEntry struct {
	name    string
	mode    fs.FileMode
	infoErr error
}

func (d stubDirEntry) Name() string {
	return d.name
}

func (d stubDirEntry) IsDir() bool {
	return d.mode.IsDir()
}

func (d stubDirEntry) Type() fs.FileMode {
	return d.mode.Type()
}

func (d stubDirEntry) Info() (fs.FileInfo, error) {
	return nil, d.infoErr
}

func TestExecScan_Visit(t *testing.T) {
	root := filepath.Join(t.TempDir(), "root")
	boom := errors.New("boom")

	testCases := []struct {
		name    string
		path    string
		entry   fs.DirEntry
		walkErr error
		want    error
		wantAny bool
	}{
		{name: "walk error", path: root, entry: stubDirEntry{}, walkErr: boom, want: boom},
		{name: "relative path", path: "rel", entry: stubDirEntry{}, wantAny: true},
		{name: "root", path: root, entry: stubDirEntry{mode: fs.ModeDir}},
		{name: "symlink", path: filepath.Join(root, "link"), entry: stubDirEntry{name: "link", mode: fs.ModeSymlink}},
		{name: "info error", path: filepath.Join(root, "f"), entry: stubDirEntry{name: "f", infoErr: boom}, want: boom},
		{name: "deep dir", path: filepath.Join(root, "a", "b", "c"), entry: stubDirEntry{name: "c", mode: fs.ModeDir}, want: filepath.SkipDir},
		{name: "shallow dir", path: filepath.Join(root, "a"), entry: stubDirEntry{name: "a", mode: fs.ModeDir}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			scan := &execScan{root: root}
			err := scan.visit(tc.path, tc.entry, tc.walkErr)
			if tc.wantAny {
				require.Error(t, err)
				return
			}
			assert.ErrorIs(t, err, tc.want)
			assert.Empty(t, scan.found)
		})
	}
}

package unpacktest_test

import (
	"io"
	"io/fs"
	"os"
	"testing"

	"github.com/diskfs/go-diskfs/backend/file"
	"github.com/diskfs/go-diskfs/filesystem/squashfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/unpack"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/unpack/unpacktest"
)

func TestBuildAppImage_WritesReadableAppImage(t *testing.T) {
	path := unpacktest.BuildAppImage(t, map[string]unpacktest.Entry{
		"AppRun":            {Mode: 0o755, Data: "#!/bin/sh\n"},
		"usr/share/doc.txt": {Data: "doc"},
		"empty":             {Mode: fs.ModeDir | 0o755},
		".DirIcon":          {Link: "usr/share/doc.txt"},
	}, 2)

	src, err := os.Open(path)
	require.NoError(t, err)
	defer src.Close()
	info, err := src.Stat()
	require.NoError(t, err)

	assert.True(t, unpack.IsAppImage(src))

	sfs, err := squashfs.Read(file.New(src, true), info.Size()-64, 64, 0)
	require.NoError(t, err)

	f, err := sfs.OpenFile("usr/share/doc.txt", os.O_RDONLY)
	require.NoError(t, err)
	body, err := io.ReadAll(f)
	require.NoError(t, err)
	assert.Equal(t, "doc", string(body))

	entries, err := sfs.ReadDir(".")
	require.NoError(t, err)
	kinds := map[string]fs.FileMode{}
	for _, e := range entries {
		info, err := e.Info()
		require.NoError(t, err)
		kinds[e.Name()] = info.Mode()
	}
	assert.Equal(t, fs.FileMode(0o755), kinds["AppRun"].Perm())
	assert.True(t, kinds["empty"].IsDir())
	assert.NotZero(t, kinds[".DirIcon"]&fs.ModeSymlink)
}

func TestBuildAppImage_WritesTypeByte(t *testing.T) {
	path := unpacktest.BuildAppImage(t, map[string]unpacktest.Entry{"AppRun": {Data: "x"}}, 1)

	head := make([]byte, 11)
	src, err := os.Open(path)
	require.NoError(t, err)
	defer src.Close()
	_, err = src.ReadAt(head, 0)
	require.NoError(t, err)

	assert.Equal(t, "\x7fELF", string(head[:4]))
	assert.Equal(t, "AI\x01", string(head[8:11]))
}

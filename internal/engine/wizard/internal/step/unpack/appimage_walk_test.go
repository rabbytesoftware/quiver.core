package unpack

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/diskfs/go-diskfs/backend/file"
	"github.com/diskfs/go-diskfs/filesystem/squashfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/unpack/unpacktest"
)

type failingInfoEntry struct {
	fs.DirEntry
}

func (failingInfoEntry) Info() (fs.FileInfo, error) {
	return nil, errors.New("info failed")
}

type namedEntry struct {
	fs.DirEntry
	name string
}

func (e namedEntry) Name() string {
	return e.name
}

type failingReaderAt struct{}

func (failingReaderAt) ReadAt([]byte, int64) (int, error) {
	return 0, errors.New("disk on fire")
}

type failingLinkEntry struct {
	fs.DirEntry
}

func (failingLinkEntry) Readlink() (string, error) {
	return "", errors.New("readlink failed")
}

func openFixtureSquashfs(
	t *testing.T,
) *squashfs.FileSystem {
	t.Helper()

	path := unpacktest.BuildAppImage(t, map[string]unpacktest.Entry{"AppRun": {Mode: 0o755, Data: "x"}}, 2)
	src, err := os.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = src.Close() })
	info, err := src.Stat()
	require.NoError(t, err)

	sfs, err := squashfs.Read(file.New(src, true), info.Size()-unpacktest.ElfHeaderSize, unpacktest.ElfHeaderSize, 0)
	require.NoError(t, err)

	return sfs
}

func openTestGuard(
	t *testing.T,
) *Guard {
	t.Helper()

	g, err := OpenGuard(context.Background(), filepath.Join(t.TempDir(), "out"), unpacktest.TestMaxBytes)
	require.NoError(t, err)
	t.Cleanup(g.Close)

	return g
}

func mapEntry(
	t *testing.T,
	mode fs.FileMode,
) fs.DirEntry {
	t.Helper()

	entries, err := fs.ReadDir(fstest.MapFS{"node": {Mode: mode}}, ".")
	require.NoError(t, err)
	require.Len(t, entries, 1)

	return entries[0]
}

func TestSquashfsWalk_Errors(t *testing.T) {
	sfs := openFixtureSquashfs(t)
	ctx := context.Background()

	testCases := []struct {
		name string
		run  func(g *Guard) error
	}{
		{name: "missing directory", run: func(g *Guard) error { return walkSquashfs(ctx, sfs, "missing", 0, g) }},
		{name: "missing file", run: func(g *Guard) error { return squashfsFile(ctx, sfs, "missing", 0o644, g) }},
		{
			name: "entry info fails",
			run: func(g *Guard) error {
				return squashfsEntry(ctx, sfs, ".", 0, failingInfoEntry{mapEntry(t, 0o644)}, g)
			},
		},
		{
			name: "symlink without a readable target",
			run:  func(g *Guard) error { return squashfsSymlink("node", mapEntry(t, fs.ModeSymlink|0o777), g) },
		},
		{
			name: "readlink fails",
			run: func(g *Guard) error {
				return squashfsSymlink("node", failingLinkEntry{mapEntry(t, fs.ModeSymlink|0o777)}, g)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Error(t, tc.run(openTestGuard(t)))
		})
	}
}

func TestSquashfsEntry_SkipsSpecialFiles(t *testing.T) {
	g := openTestGuard(t)

	err := squashfsEntry(context.Background(), openFixtureSquashfs(t), ".", 0, mapEntry(t, fs.ModeNamedPipe|0o644), g)

	require.NoError(t, err)
	assert.Empty(t, g.TopLevel())
}

func TestSquashfsEntry_RejectsUnsafeNames(t *testing.T) {
	sfs := openFixtureSquashfs(t)

	testCases := []struct {
		name  string
		entry string
	}{
		{name: "empty", entry: ""},
		{name: "dot", entry: "."},
		{name: "dot dot", entry: ".."},
		{name: "slash", entry: "a/b"},
		{name: "backslash", entry: `a\b`},
		{name: "nul", entry: "a\x00b"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := openTestGuard(t)
			e := namedEntry{DirEntry: mapEntry(t, fs.ModeDir|0o755), name: tc.entry}

			err := squashfsEntry(context.Background(), sfs, "usr", 0, e, g)

			require.ErrorIs(t, err, ErrEscape)
			assert.Empty(t, g.TopLevel())
		})
	}
}

func TestWalkSquashfs_DepthCap(t *testing.T) {
	sfs := openFixtureSquashfs(t)

	testCases := []struct {
		name    string
		depth   int
		wantErr bool
	}{
		{name: "at the cap", depth: maxSquashfsDepth},
		{name: "past the cap", depth: maxSquashfsDepth + 1, wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := walkSquashfs(context.Background(), sfs, ".", tc.depth, openTestGuard(t))

			if !tc.wantErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), "nesting")
		})
	}
}

func TestSquashfsEntry_DirectoryAtTheCapStopsDescent(t *testing.T) {
	g := openTestGuard(t)
	e := namedEntry{DirEntry: mapEntry(t, fs.ModeDir|0o755), name: "usr"}

	err := squashfsEntry(context.Background(), openFixtureSquashfs(t), ".", maxSquashfsDepth, e, g)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "nesting")
}

func TestHasSquashfsMagic_ReadErrors(t *testing.T) {
	testCases := []struct {
		name    string
		src     io.ReaderAt
		want    bool
		wantErr bool
	}{
		{name: "magic present", src: bytes.NewReader([]byte("xxhsqs")), want: true},
		{name: "other bytes", src: bytes.NewReader([]byte("xxnope")), want: false},
		{name: "short read at the end", src: bytes.NewReader([]byte("xxhs")), want: false},
		{name: "past the end", src: bytes.NewReader([]byte("x")), want: false},
		{name: "read error", src: failingReaderAt{}, wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ok, err := hasSquashfsMagic(tc.src, 2)

			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "disk on fire")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, ok)
		})
	}
}

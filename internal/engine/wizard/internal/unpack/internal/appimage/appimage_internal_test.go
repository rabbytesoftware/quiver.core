package appimage

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/diskfs/go-diskfs/backend/file"
	"github.com/diskfs/go-diskfs/filesystem/squashfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/guard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/mocks"
)

func TestReadAppImageMeta_FullMetadata(t *testing.T) {
	icon := "usr/share/icons/hicolor/1024x1024/apps/bruno.png"
	dir := writeAppDir(t, map[string]string{
		"AppRun":        "run",
		"bruno.desktop": "[Desktop Entry]\nName=Bruno\nExec=AppRun --no-sandbox %U\nIcon=bruno\n",
		icon:            "png",
	}, map[string]string{".DirIcon": icon})

	meta, err := ReadMeta(dir)

	require.NoError(t, err)
	assert.Equal(t, Meta{Name: "Bruno", Args: []string{"--no-sandbox"}, Icon: icon}, meta)
}

func TestReadAppImageMeta_NoDesktopFile(t *testing.T) {
	dir := writeAppDir(t, map[string]string{"AppRun": "run", "bruno.png": "png"}, nil)

	meta, err := ReadMeta(dir)

	require.NoError(t, err)
	assert.Empty(t, meta.Name)
	assert.Empty(t, meta.Args)
	assert.Empty(t, meta.Icon)
}

func TestReadAppImageMeta_NoDesktopFileFallsBackToDirIcon(t *testing.T) {
	dir := writeAppDir(t, map[string]string{"AppRun": "run", ".DirIcon": "png"}, nil)

	meta, err := ReadMeta(dir)

	require.NoError(t, err)
	assert.Equal(t, Meta{Icon: ".DirIcon"}, meta)
}

func TestReadAppImageMeta_OversizedDesktopFileIsIgnored(t *testing.T) {
	testCases := []struct {
		name     string
		padding  int
		wantName string
	}{
		{name: "exactly at the cap is read", padding: maxDesktopFileBytes, wantName: "Big"},
		{name: "one byte over the cap is ignored", padding: maxDesktopFileBytes + 1, wantName: ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			head := "[Desktop Entry]\nName=Big\n"
			body := head + strings.Repeat("#", tc.padding-len(head))
			dir := writeAppDir(t, map[string]string{"a.desktop": body, ".DirIcon": "png"}, nil)

			meta, err := ReadMeta(dir)

			require.NoError(t, err)
			assert.Equal(t, tc.wantName, meta.Name)
			assert.Empty(t, meta.Args)
			assert.Equal(t, ".DirIcon", meta.Icon)
		})
	}
}

func TestReadAppImageMeta_VendorArgs(t *testing.T) {
	testCases := []struct {
		name  string
		exec  string
		links map[string]string
		want  []string
	}{
		{name: "apprun keeps args", exec: "AppRun --no-sandbox %U", want: []string{"--no-sandbox"}},
		{name: "relative regular file keeps args", exec: "usr/bin/app --flag %f", want: []string{"--flag"}},
		{name: "relative symlink to a file inside keeps args", exec: "linked --flag", links: map[string]string{"linked": "usr/bin/app"}, want: []string{"--flag"}},
		{name: "bare name that is not a file drops args", exec: "prismlauncher --x %U"},
		{name: "absolute program drops args", exec: "/usr/bin/app --flag"},
		{name: "escaping program drops args", exec: "../app --flag"},
		{name: "directory program drops args", exec: "usr/bin --flag"},
		{name: "empty exec has no args", exec: ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeAppDir(t, map[string]string{
				"AppRun":      "run",
				"usr/bin/app": "bin",
				"app.desktop": "[Desktop Entry]\nName=App\nExec=" + tc.exec + "\n",
			}, tc.links)
			require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(dir), "app"), []byte("x"), 0o644))

			meta, err := ReadMeta(dir)

			require.NoError(t, err)
			if tc.want == nil {
				assert.Empty(t, meta.Args)
				return
			}
			assert.Equal(t, tc.want, meta.Args)
		})
	}
}

func TestReadAppImageMeta_FirstDesktopFileByName(t *testing.T) {
	dir := writeAppDir(t, map[string]string{
		"b.desktop":           "[Desktop Entry]\nName=Second\n",
		"a.desktop":           "[Desktop Entry]\nName=First\n",
		"0.desktop/inner":     "not a file",
		"notes.txt":           "Name=Nope",
		"usr/share/x.desktop": "[Desktop Entry]\nName=Nested\n",
	}, map[string]string{"00.desktop": "missing.desktop"})

	meta, err := ReadMeta(dir)

	require.NoError(t, err)
	assert.Equal(t, "First", meta.Name)
}

func TestReadAppImageMeta_MissingAppDir(t *testing.T) {
	_, err := ReadMeta(filepath.Join(t.TempDir(), "missing"))

	require.Error(t, err)
}

func TestReadAppImageMeta_UnreadableDesktopFile(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permission bits do not block reads here")
	}
	dir := writeAppDir(t, map[string]string{"a.desktop": "[Desktop Entry]\nName=A\n"}, nil)
	require.NoError(t, os.Chmod(filepath.Join(dir, "a.desktop"), 0o000))

	_, err := ReadMeta(dir)

	require.Error(t, err)
}

func elf64WithProgram(
	bo binary.ByteOrder,
	progOff uint64,
	progSize uint64,
) []byte {
	data := make([]byte, 256)
	copy(data, mocks.ElfHeader(2))
	data[5] = 1
	if bo == binary.BigEndian {
		data[5] = 2
	}
	bo.PutUint64(data[0x20:], 64)
	bo.PutUint64(data[0x28:], 120)
	bo.PutUint16(data[0x36:], 56)
	bo.PutUint16(data[0x38:], 1)
	bo.PutUint16(data[0x3A:], 64)
	bo.PutUint16(data[0x3C:], 1)
	bo.PutUint64(data[64+0x08:], progOff)
	bo.PutUint64(data[64+0x20:], progSize)

	return data
}

func elf32WithProgram(
	progOff uint32,
	progSize uint32,
) []byte {
	data := make([]byte, 128)
	copy(data, "\x7fELF\x01\x01\x01")
	le := binary.LittleEndian
	le.PutUint32(data[0x1C:], 52)
	le.PutUint32(data[0x20:], 52)
	le.PutUint16(data[0x2A:], 32)
	le.PutUint16(data[0x2C:], 1)
	le.PutUint16(data[0x2E:], 40)
	le.PutUint16(data[0x30:], 2)
	le.PutUint32(data[52+0x04:], progOff)
	le.PutUint32(data[52+0x10:], progSize)

	return data
}

func TestSquashfsOffset_HeaderOnlyIsEndOfSectionTable(t *testing.T) {
	off, err := squashfsOffset(bytes.NewReader(mocks.ElfHeader(2)))

	require.NoError(t, err)
	assert.Equal(t, int64(64), off)
}

func TestSquashfsOffset_ProgramHeaders(t *testing.T) {
	testCases := []struct {
		name string
		data []byte
		want int64
	}{
		{name: "64-bit segment past the section table raises the offset", data: elf64WithProgram(binary.LittleEndian, 100, 400), want: 500},
		{name: "64-bit segment before the section table keeps it", data: elf64WithProgram(binary.LittleEndian, 0, 10), want: 184},
		{name: "64-bit big endian", data: elf64WithProgram(binary.BigEndian, 100, 400), want: 500},
		{name: "32-bit segment past the section table raises the offset", data: elf32WithProgram(100, 300), want: 400},
		{name: "32-bit segment before the section table keeps it", data: elf32WithProgram(0, 10), want: 132},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			off, err := squashfsOffset(bytes.NewReader(tc.data))

			require.NoError(t, err)
			assert.Equal(t, tc.want, off)
		})
	}
}

func TestSquashfsOffset_Errors(t *testing.T) {
	truncated64 := elf64WithProgram(binary.LittleEndian, 0, 0)
	binary.LittleEndian.PutUint64(truncated64[0x20:], 1<<20)
	truncated32 := elf32WithProgram(0, 0)
	binary.LittleEndian.PutUint32(truncated32[0x1C:], 1<<20)
	huge := mocks.ElfHeader(2)
	binary.LittleEndian.PutUint64(huge[0x28:], 1<<63)
	hugeProg := elf64WithProgram(binary.LittleEndian, 0, 0)
	binary.LittleEndian.PutUint64(hugeProg[0x20:], 1<<63)
	badClass := mocks.ElfHeader(2)
	badClass[4] = 7
	badData := mocks.ElfHeader(2)
	badData[5] = 9

	testCases := []struct {
		name string
		data []byte
	}{
		{name: "not an elf file", data: []byte("MZ this is not an elf file at all")},
		{name: "unknown class", data: badClass},
		{name: "shorter than the elf ident", data: []byte("\x7fELF")},
		{name: "unknown data encoding", data: badData},
		{name: "truncated 64-bit header", data: mocks.ElfHeader(2)[:40]},
		{name: "truncated 32-bit header", data: []byte("\x7fELF\x01\x01\x01\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")},
		{name: "64-bit program header out of range", data: truncated64},
		{name: "32-bit program header out of range", data: truncated32},
		{name: "offset beyond int64", data: huge},
		{name: "program header offset beyond int64", data: hugeProg},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := squashfsOffset(bytes.NewReader(tc.data))

			require.Error(t, err)
		})
	}
}

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

var errDiskOnFire = errors.New("disk on fire")

type failingReaderAt struct{}

func (failingReaderAt) ReadAt([]byte, int64) (int, error) {
	return 0, errDiskOnFire
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

	path := mocks.BuildAppImage(t, map[string]mocks.Entry{"AppRun": {Mode: 0o755, Data: "x"}}, 2)
	src, err := os.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = src.Close() })
	info, err := src.Stat()
	require.NoError(t, err)

	sfs, err := squashfs.Read(file.New(src, true), info.Size()-mocks.ElfHeaderSize, mocks.ElfHeaderSize, 0)
	require.NoError(t, err)

	return sfs
}

func openTestGuard(
	t *testing.T,
) *guard.Guard {
	t.Helper()

	g, err := guard.Open(context.Background(), filepath.Join(t.TempDir(), "out"), mocks.TestMaxBytes)
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
		run  func(g *guard.Guard) error
	}{
		{name: "missing directory", run: func(g *guard.Guard) error { return walkSquashfs(ctx, sfs, "missing", 0, g) }},
		{name: "missing file", run: func(g *guard.Guard) error { return squashfsFile(ctx, sfs, "missing", 0o644, g) }},
		{
			name: "entry info fails",
			run: func(g *guard.Guard) error {
				return squashfsEntry(ctx, sfs, ".", 0, failingInfoEntry{mapEntry(t, 0o644)}, g)
			},
		},
		{
			name: "symlink without a readable target",
			run:  func(g *guard.Guard) error { return squashfsSymlink("node", mapEntry(t, fs.ModeSymlink|0o777), g) },
		},
		{
			name: "readlink fails",
			run: func(g *guard.Guard) error {
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
		{name: "slash", entry: "a/b"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := openTestGuard(t)
			e := namedEntry{DirEntry: mapEntry(t, fs.ModeDir|0o755), name: tc.entry}

			err := squashfsEntry(context.Background(), sfs, "usr", 0, e, g)

			require.ErrorIs(t, err, models.ErrEscape)
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
			g := openTestGuard(t)

			err := walkSquashfs(context.Background(), sfs, ".", tc.depth, g)

			if !tc.wantErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Empty(t, g.TopLevel())
		})
	}
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
				require.ErrorIs(t, err, errDiskOnFire)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, ok)
		})
	}
}

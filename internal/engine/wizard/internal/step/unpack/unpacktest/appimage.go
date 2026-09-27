package unpacktest

import (
	"bytes"
	"encoding/binary"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/diskfs/go-diskfs/backend/file"
	"github.com/diskfs/go-diskfs/filesystem/squashfs"
	"github.com/stretchr/testify/require"
)

const (
	ElfHeaderSize   = 64
	defaultFileMode = 0o644
	workspaceDirMod = 0o755

	maxPlaceholderBits = 16
)

type Entry struct {
	Mode fs.FileMode
	Data string
	Link string
}

func BuildAppImage(
	t testing.TB,
	entries map[string]Entry,
	typeByte byte,
) string {
	t.Helper()

	dir := t.TempDir()
	image := buildSquashfs(t, dir, entries)
	path := filepath.Join(dir, "app.AppImage")
	require.NoError(t, os.WriteFile(path, append(ElfHeader(typeByte), image...), 0o600))

	return path
}

func ElfHeader(
	typeByte byte,
) []byte {
	hdr := make([]byte, ElfHeaderSize)
	copy(hdr, "\x7fELF")
	hdr[4] = 2
	hdr[5] = 1
	hdr[6] = 1
	copy(hdr[8:], []byte{'A', 'I', typeByte})
	binary.LittleEndian.PutUint32(hdr[0x14:], 1)
	binary.LittleEndian.PutUint64(hdr[0x28:], ElfHeaderSize)
	binary.LittleEndian.PutUint16(hdr[0x34:], ElfHeaderSize)
	binary.LittleEndian.PutUint16(hdr[0x3A:], ElfHeaderSize)

	return hdr
}

func buildSquashfs(
	t testing.TB,
	dir string,
	entries map[string]Entry,
) []byte {
	t.Helper()

	path := filepath.Join(dir, "image.sqfs")
	f, err := os.Create(path) //nolint:gosec
	require.NoError(t, err)
	defer f.Close() //nolint:errcheck

	sfs, err := squashfs.Create(file.New(f, false), 0, 0, 0)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sfs.Close() })

	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	slices.Sort(names)

	links := make(map[string]string)
	for _, name := range names {
		e := entries[name]
		if e.Link != "" {
			placeholder := linkPlaceholder(t, e.Link, links)
			links[placeholder] = e.Link
			e.Link = placeholder
		}
		writeEntry(t, filepath.Join(sfs.Workspace(), filepath.FromSlash(name)), e)
	}

	t.Chdir(sfs.Workspace())
	require.NoError(t, sfs.Finalize(squashfs.FinalizeOptions{NoCompressInodes: true}))

	data, err := os.ReadFile(path) //nolint:gosec
	require.NoError(t, err)

	for placeholder, target := range links {
		data = patchLinkTarget(t, data, placeholder, target)
	}

	return data
}

func linkPlaceholder(
	t testing.TB,
	target string,
	taken map[string]string,
) string {
	t.Helper()

	for bits := range 1 << min(len(target), maxPlaceholderBits) {
		candidate := dotSlashPattern(len(target), bits)
		if _, used := taken[candidate]; !used {
			return candidate
		}
	}
	require.FailNow(t, "no unique placeholder for link target", target)

	return ""
}

func dotSlashPattern(
	n int,
	bits int,
) string {
	pattern := make([]byte, n)
	pattern[0] = '.'
	for i := 1; i < n; i++ {
		pattern[i] = '/'
		if pattern[i-1] == '/' && bits>>(i-1)&1 == 1 {
			pattern[i] = '.'
		}
	}

	return string(pattern)
}

func patchLinkTarget(
	t testing.TB,
	image []byte,
	placeholder string,
	target string,
) []byte {
	t.Helper()

	size := binary.LittleEndian.AppendUint32(nil, uint32(len(target))) //nolint:gosec
	needle := append(slices.Clone(size), placeholder...)
	require.Equal(t, 1, bytes.Count(image, needle), "symlink inode for %q", target)

	return bytes.Replace(image, needle, append(size, target...), 1)
}

func writeEntry(
	t testing.TB,
	path string,
	e Entry,
) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), workspaceDirMod))

	switch {
	case e.Link != "":
		require.NoError(t, os.Symlink(e.Link, path))
	case e.Mode.IsDir():
		require.NoError(t, os.MkdirAll(path, e.Mode.Perm()))
	default:
		perm := e.Mode.Perm()
		if perm == 0 {
			perm = defaultFileMode
		}
		require.NoError(t, os.WriteFile(path, []byte(e.Data), perm))
		require.NoError(t, os.Chmod(path, perm))
	}
}

package guard_test

import (
	"archive/tar"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/archive"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/guard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/unpacktest"
)

func runUnpack(
	t *testing.T,
	maxBytes int64,
	from string,
	to string,
	timeout time.Duration,
) error {
	t.Helper()

	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	src, err := os.Open(from)
	require.NoError(t, err)
	defer src.Close()

	info, err := src.Stat()
	require.NoError(t, err)

	kind, err := archive.Detect(src, info.Size())
	if err != nil {
		return err
	}

	g, err := guard.Open(context.Background(), to, maxBytes)
	if err != nil {
		return err
	}
	defer g.Close()

	return errors.Join(kind.Extract(ctx, src, info.Size(), g), g.Verify())
}

func TestArchive_Extract_SizeCeiling(t *testing.T) {
	testCases := []struct {
		name     string
		file     string
		build    func(t *testing.T) []byte
		maxBytes int64
		wantErr  bool
	}{
		{
			name: "cumulative tar entries over the limit",
			file: "a.tar",
			build: func(t *testing.T) []byte {
				return unpacktest.TarBytes(t,
					unpacktest.TarEntry{Name: "one", Body: "123456", Mode: 0o644, Flag: tar.TypeReg},
					unpacktest.TarEntry{Name: "two", Body: "123456", Mode: 0o644, Flag: tar.TypeReg},
				)
			},
			maxBytes: 10,
			wantErr:  true,
		},
		{
			name: "cumulative tar entries exactly at the limit",
			file: "a.tar",
			build: func(t *testing.T) []byte {
				return unpacktest.TarBytes(t,
					unpacktest.TarEntry{Name: "one", Body: "12345", Mode: 0o644, Flag: tar.TypeReg},
					unpacktest.TarEntry{Name: "two", Body: "12345", Mode: 0o644, Flag: tar.TypeReg},
				)
			},
			maxBytes: 10,
		},
		{
			name: "zip bomb",
			file: "a.zip",
			build: func(t *testing.T) []byte {
				return unpacktest.ZipBytes(t, unpacktest.ZipEntry{Name: "big", Body: strings.Repeat("0", 4096), Mode: 0o644})
			},
			maxBytes: 1024,
			wantErr:  true,
		},
		{
			name:     "single file bomb",
			file:     "big.gz",
			build:    func(t *testing.T) []byte { return unpacktest.GzipBytes(t, []byte(strings.Repeat("0", 4096))) },
			maxBytes: 1024,
			wantErr:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			from := unpacktest.WriteArchive(t, dir, tc.file, tc.build(t))

			err := runUnpack(t, tc.maxBytes, from, filepath.Join(dir, "out"), 0)

			if !tc.wantErr {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, models.ErrTooLarge)
		})
	}
}

func TestGuard_EntryCeiling(t *testing.T) {
	entries := []unpacktest.TarEntry{
		{Name: "dir", Mode: 0o755, Flag: tar.TypeDir},
		{Name: "dir/file", Body: "x", Mode: 0o644, Flag: tar.TypeReg},
		{Name: "dir/soft", Link: "file", Flag: tar.TypeSymlink},
		{Name: "dir/hard", Link: "dir/file", Flag: tar.TypeLink},
	}
	testCases := []struct {
		name    string
		limit   int
		wantErr bool
		missing string
	}{
		{name: "every entry kind within the limit extracts", limit: 4},
		{name: "one entry past the limit stops extraction", limit: 3, wantErr: true, missing: "dir/hard"},
		{name: "the limit stops before the first file", limit: 1, wantErr: true, missing: "dir/file"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			to := filepath.Join(t.TempDir(), "out")

			_, err := extractTarWithGuard(t, to, entries, guard.LimitEntries(tc.limit))

			if !tc.wantErr {
				require.NoError(t, err)
				assert.FileExists(t, filepath.Join(to, "dir", "hard"))
				return
			}
			require.ErrorIs(t, err, models.ErrTooMany)
			_, statErr := os.Lstat(filepath.Join(to, filepath.FromSlash(tc.missing)))
			assert.ErrorIs(t, statErr, os.ErrNotExist)
		})
	}
}

func TestGuard_EntryCeilingIsOneMillion(t *testing.T) {
	assert.Equal(t, 1_000_000, guard.MaxEntries)
}

func TestArchive_Extract_SizeCeilingRemovesPartialFile(t *testing.T) {
	dir := t.TempDir()
	from := unpacktest.WriteArchive(t, dir, "big.gz", unpacktest.GzipBytes(t, []byte(strings.Repeat("0", 4096))))
	to := filepath.Join(dir, "out")

	err := runUnpack(t, 1024, from, to, 0)

	require.ErrorIs(t, err, models.ErrTooLarge)
	_, statErr := os.Lstat(filepath.Join(to, "big"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestArchive_Extract_HonoursTimeout(t *testing.T) {
	dir := t.TempDir()
	from := unpacktest.WriteArchive(t, dir, "tool.gz", unpacktest.GzipBytes(t, []byte("tool")))

	err := runUnpack(t, unpacktest.TestMaxBytes, from, filepath.Join(dir, "out"), time.Nanosecond)

	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestArchive_Extract_RejectsLinksThatEscapeOnDisk(t *testing.T) {
	testCases := []struct {
		name     string
		entries  []unpacktest.TarEntry
		leftover string
	}{
		{
			name: "link placed through an in-archive symlink",
			entries: []unpacktest.TarEntry{
				{Name: "d/", Mode: 0o755, Flag: tar.TypeDir},
				{Name: "d/up", Mode: 0o777, Flag: tar.TypeSymlink, Link: ".."},
				{Name: "d/up/esc", Mode: 0o777, Flag: tar.TypeSymlink, Link: "../outside"},
			},
			leftover: "esc",
		},
		{
			name: "dangling link through a symlink chain",
			entries: []unpacktest.TarEntry{
				{Name: "a/b/c/", Mode: 0o755, Flag: tar.TypeDir},
				{Name: "a/b/c/l", Mode: 0o777, Flag: tar.TypeSymlink, Link: "../../.."},
				{Name: "y", Mode: 0o777, Flag: tar.TypeSymlink, Link: "a/b/c/l/../../../../outside/nd"},
			},
			leftover: "y",
		},
		{
			name: "symlink loop",
			entries: []unpacktest.TarEntry{
				{Name: "loop", Mode: 0o777, Flag: tar.TypeSymlink, Link: "loop"},
			},
			leftover: "loop",
		},
		{
			name: "link target traversing an in-archive symlink",
			entries: []unpacktest.TarEntry{
				{Name: "self", Mode: 0o777, Flag: tar.TypeSymlink, Link: "."},
				{Name: "esc", Mode: 0o777, Flag: tar.TypeSymlink, Link: "self/.."},
			},
			leftover: "esc",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "outside"), []byte("secret"), 0o600))
			from := unpacktest.WriteArchive(t, dir, "a.tar", unpacktest.TarBytes(t, tc.entries...))
			to := filepath.Join(dir, "out")

			err := runUnpack(t, unpacktest.TestMaxBytes, from, to, 0)

			require.ErrorIs(t, err, models.ErrEscape)
			_, statErr := os.Lstat(filepath.Join(to, tc.leftover))
			assert.ErrorIs(t, statErr, os.ErrNotExist)
		})
	}
}

func TestArchive_Extract_RefusesToWriteThroughEscapingSymlink(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	to := filepath.Join(dir, "out")
	require.NoError(t, os.MkdirAll(to, 0o755))
	require.NoError(t, os.Symlink(outside, filepath.Join(to, "link")))
	from := unpacktest.WriteArchive(t, dir, "a.tar", unpacktest.TarBytes(t,
		unpacktest.TarEntry{Name: "link/pwn.txt", Body: "pwn", Mode: 0o644, Flag: tar.TypeReg},
	))

	err := runUnpack(t, unpacktest.TestMaxBytes, from, to, 0)

	require.Error(t, err)
	_, statErr := os.Lstat(filepath.Join(outside, "pwn.txt"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestArchive_Extract_KeepsDanglingLinkInsideDestination(t *testing.T) {
	dir := t.TempDir()
	from := unpacktest.WriteArchive(t, dir, "a.tar", unpacktest.TarBytes(t,
		unpacktest.TarEntry{Name: "lib/current", Mode: 0o777, Flag: tar.TypeSymlink, Link: "../versions/missing"},
	))
	to := filepath.Join(dir, "out")

	require.NoError(t, runUnpack(t, unpacktest.TestMaxBytes, from, to, 0))

	target, err := os.Readlink(filepath.Join(to, "lib", "current"))
	require.NoError(t, err)
	assert.Equal(t, "../versions/missing", target)
}

func TestGuard_Verify_RemovesEveryEscapingLink(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "outside"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "outside", "secret"), []byte("secret"), 0o600))
	from := unpacktest.WriteArchive(t, dir, "a.tar", unpacktest.TarBytes(t,
		unpacktest.TarEntry{Name: "a/b/c/", Mode: 0o755, Flag: tar.TypeDir},
		unpacktest.TarEntry{Name: "a/b/c/l", Mode: 0o777, Flag: tar.TypeSymlink, Link: "../../.."},
		unpacktest.TarEntry{Name: "e1", Mode: 0o777, Flag: tar.TypeSymlink, Link: "a/b/c/l/../../../../outside"},
		unpacktest.TarEntry{Name: "e2", Mode: 0o777, Flag: tar.TypeSymlink, Link: "a/b/c/l/../../../../outside/secret"},
	))
	to := filepath.Join(dir, "deep", "out")

	err := runUnpack(t, unpacktest.TestMaxBytes, from, to, 0)

	require.ErrorIs(t, err, models.ErrEscape)
	for _, name := range []string{"e1", "e2"} {
		_, statErr := os.Lstat(filepath.Join(to, name))
		assert.ErrorIs(t, statErr, os.ErrNotExist, name)
	}
}

func TestGuard_OpenGuard_AppliesOptions(t *testing.T) {
	dir := t.TempDir()
	called := false
	opt := func(*guard.Guard) { called = true }

	g, err := guard.Open(context.Background(), filepath.Join(dir, "out"), unpacktest.TestMaxBytes, opt)
	require.NoError(t, err)
	defer g.Close()

	assert.True(t, called)
}

func TestGuard_TopLevel_ReturnsSortedUniqueNames(t *testing.T) {
	dir := t.TempDir()
	from := unpacktest.WriteArchive(t, dir, "a.tar", unpacktest.TarBytes(t,
		unpacktest.TarEntry{Name: "b/x", Body: "x", Mode: 0o644, Flag: tar.TypeReg},
		unpacktest.TarEntry{Name: "a/y", Body: "y", Mode: 0o644, Flag: tar.TypeReg},
		unpacktest.TarEntry{Name: "a/z", Body: "z", Mode: 0o644, Flag: tar.TypeReg},
		unpacktest.TarEntry{Name: "top.txt", Body: "t", Mode: 0o644, Flag: tar.TypeReg},
	))
	to := filepath.Join(dir, "out")

	src, err := os.Open(from)
	require.NoError(t, err)
	defer src.Close()
	info, err := src.Stat()
	require.NoError(t, err)
	kind, err := archive.Detect(src, info.Size())
	require.NoError(t, err)

	g, err := guard.Open(context.Background(), to, unpacktest.TestMaxBytes)
	require.NoError(t, err)
	defer g.Close()
	require.NoError(t, kind.Extract(context.Background(), src, info.Size(), g))

	assert.Equal(t, []string{"a", "b", "top.txt"}, g.TopLevel())
}

func TestArchive_Extract_WriteThroughArchiveEscapingSymlinkIsRefused(t *testing.T) {
	dir := t.TempDir()
	from := unpacktest.WriteArchive(t, dir, "a.tar", unpacktest.TarBytes(t,
		unpacktest.TarEntry{Name: "self", Mode: 0o777, Flag: tar.TypeSymlink, Link: "."},
		unpacktest.TarEntry{Name: "esc", Mode: 0o777, Flag: tar.TypeSymlink, Link: "self/.."},
		unpacktest.TarEntry{Name: "esc/pwn.txt", Body: "pwn", Mode: 0o644, Flag: tar.TypeReg},
	))
	to := filepath.Join(dir, "out")

	err := runUnpack(t, unpacktest.TestMaxBytes, from, to, 0)

	require.Error(t, err)
	assert.ErrorIs(t, err, models.ErrEscape)
	_, statErr := os.Lstat(filepath.Join(dir, "pwn.txt"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
	_, statErr = os.Lstat(filepath.Join(to, "esc"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func extractTarWithGuard(
	t *testing.T,
	to string,
	entries []unpacktest.TarEntry,
	opts ...guard.Option,
) (*guard.Guard, error) {
	t.Helper()

	from := unpacktest.WriteArchive(t, t.TempDir(), "a.tar", unpacktest.TarBytes(t, entries...))
	src, err := os.Open(from)
	require.NoError(t, err)
	defer src.Close()
	info, err := src.Stat()
	require.NoError(t, err)
	kind, err := archive.Detect(src, info.Size())
	require.NoError(t, err)

	g, err := guard.Open(context.Background(), to, unpacktest.TestMaxBytes, opts...)
	require.NoError(t, err)
	t.Cleanup(g.Close)

	return g, kind.Extract(context.Background(), src, info.Size(), g)
}

func TestSkipEscapingLinks_SymlinkSkipsEscapingTargets(t *testing.T) {
	to := filepath.Join(t.TempDir(), "out")

	_, err := extractTarWithGuard(t, to, []unpacktest.TarEntry{
		{Name: "abs", Mode: 0o777, Flag: tar.TypeSymlink, Link: "/home/runner/x.png"},
		{Name: "up", Mode: 0o777, Flag: tar.TypeSymlink, Link: "../../etc"},
		{Name: "d/self", Mode: 0o777, Flag: tar.TypeSymlink, Link: ".."},
		{Name: "d/self/esc", Mode: 0o777, Flag: tar.TypeSymlink, Link: "../outside"},
		{Name: "file", Body: "x", Mode: 0o644, Flag: tar.TypeReg},
		{Name: "inside", Mode: 0o777, Flag: tar.TypeSymlink, Link: "file"},
	}, guard.SkipEscapingLinks())

	require.NoError(t, err)
	for _, name := range []string{"abs", "up", "esc"} {
		_, statErr := os.Lstat(filepath.Join(to, name))
		assert.ErrorIs(t, statErr, os.ErrNotExist, name)
	}
	target, err := os.Readlink(filepath.Join(to, "inside"))
	require.NoError(t, err)
	assert.Equal(t, "file", target)
}

func TestSkipEscapingLinks_SymlinkStillRejectsEmptyTarget(t *testing.T) {
	to := filepath.Join(t.TempDir(), "out")

	_, err := extractTarWithGuard(t, to, []unpacktest.TarEntry{
		{Name: "empty", Mode: 0o777, Flag: tar.TypeSymlink, Link: ""},
	}, guard.SkipEscapingLinks())

	require.Error(t, err)
	_, statErr := os.Lstat(filepath.Join(to, "empty"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestSkipEscapingLinks_VerifyRemovesEscapingLinksSilently(t *testing.T) {
	dir := t.TempDir()
	to := filepath.Join(dir, "deep", "out")

	g, err := extractTarWithGuard(t, to, []unpacktest.TarEntry{
		{Name: "a/b/c/", Mode: 0o755, Flag: tar.TypeDir},
		{Name: "a/b/c/l", Mode: 0o777, Flag: tar.TypeSymlink, Link: "../../.."},
		{Name: "e1", Mode: 0o777, Flag: tar.TypeSymlink, Link: "a/b/c/l/../../../../outside"},
		{Name: "kept", Mode: 0o777, Flag: tar.TypeSymlink, Link: "a/b"},
	}, guard.SkipEscapingLinks())
	require.NoError(t, err)

	require.NoError(t, g.Verify())

	_, statErr := os.Lstat(filepath.Join(to, "e1"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
	_, statErr = os.Lstat(filepath.Join(to, "kept"))
	assert.NoError(t, statErr)
}

func TestSkipEscapingLinks_WithoutOptionVerifyStillFails(t *testing.T) {
	dir := t.TempDir()
	to := filepath.Join(dir, "deep", "out")

	g, err := extractTarWithGuard(t, to, []unpacktest.TarEntry{
		{Name: "a/b/c/", Mode: 0o755, Flag: tar.TypeDir},
		{Name: "a/b/c/l", Mode: 0o777, Flag: tar.TypeSymlink, Link: "../../.."},
		{Name: "e1", Mode: 0o777, Flag: tar.TypeSymlink, Link: "a/b/c/l/../../../../outside"},
	})
	require.NoError(t, err)

	require.ErrorIs(t, g.Verify(), models.ErrEscape)
}

func TestSkipEscapingLinks_SkippedLinkLeavesTreeUntouched(t *testing.T) {
	to := filepath.Join(t.TempDir(), "out")

	_, err := extractTarWithGuard(t, to, []unpacktest.TarEntry{
		{Name: "esc", Body: "keep", Mode: 0o644, Flag: tar.TypeReg},
		{Name: "d/self", Mode: 0o777, Flag: tar.TypeSymlink, Link: ".."},
		{Name: "d/self/esc", Mode: 0o777, Flag: tar.TypeSymlink, Link: "../outside"},
		{Name: "d/self/new/deeper/x", Mode: 0o777, Flag: tar.TypeSymlink, Link: "../../../../outside"},
	}, guard.SkipEscapingLinks())

	require.NoError(t, err)
	assert.Equal(t, "keep", unpacktest.ReadString(t, filepath.Join(to, "esc")))
	_, statErr := os.Lstat(filepath.Join(to, "new"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestGuard_Symlink_NewParentsResolvedBeforePlacing(t *testing.T) {
	to := filepath.Join(t.TempDir(), "out")

	_, err := extractTarWithGuard(t, to, []unpacktest.TarEntry{
		{Name: "a/b/c/l", Mode: 0o777, Flag: tar.TypeSymlink, Link: "../../../file"},
	})

	require.NoError(t, err)
	target, err := os.Readlink(filepath.Join(to, "a", "b", "c", "l"))
	require.NoError(t, err)
	assert.Equal(t, "../../../file", target)
}

func TestGuard_Symlink_UnderRegularFileFails(t *testing.T) {
	to := filepath.Join(t.TempDir(), "out")

	_, err := extractTarWithGuard(t, to, []unpacktest.TarEntry{
		{Name: "f", Body: "x", Mode: 0o644, Flag: tar.TypeReg},
		{Name: "f/l", Mode: 0o777, Flag: tar.TypeSymlink, Link: "x"},
	}, guard.SkipEscapingLinks())

	require.Error(t, err)
	assert.NotErrorIs(t, err, models.ErrEscape)
	assert.Equal(t, "x", unpacktest.ReadString(t, filepath.Join(to, "f")))
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("boom")
}

func openGuardWithFile(
	t *testing.T,
	opts ...guard.Option,
) (*guard.Guard, string) {
	t.Helper()

	dest := filepath.Join(t.TempDir(), "out")
	g, err := guard.Open(context.Background(), dest, unpacktest.TestMaxBytes, opts...)
	require.NoError(t, err)
	t.Cleanup(g.Close)
	require.NoError(t, g.File(context.Background(), "plain", 0o644, strings.NewReader("x")))

	return g, dest
}

func TestOpen_DestinationUnderRegularFileFails(t *testing.T) {
	blocker := unpacktest.WriteArchive(t, t.TempDir(), "file", []byte("x"))

	_, err := guard.Open(context.Background(), filepath.Join(blocker, "out"), unpacktest.TestMaxBytes)

	assert.Error(t, err)
}

func TestGuard_Dir_Table(t *testing.T) {
	testCases := []struct {
		name    string
		dir     string
		wantErr error
		wantAny bool
	}{
		{name: "absolute name", dir: "/abs", wantErr: models.ErrEscape},
		{name: "parent traversal", dir: "../up", wantErr: models.ErrEscape},
		{name: "destination itself is a no-op", dir: "."},
		{name: "under a regular file", dir: "plain/sub", wantAny: true},
		{name: "plain directory", dir: "sub/deeper"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g, dest := openGuardWithFile(t)

			err := g.Dir(tc.dir, 0o755)

			switch {
			case tc.wantErr != nil:
				assert.ErrorIs(t, err, tc.wantErr)
			case tc.wantAny:
				assert.Error(t, err)
			default:
				require.NoError(t, err)
				assert.DirExists(t, filepath.Join(dest, filepath.FromSlash(tc.dir)))
			}
		})
	}
}

func TestGuard_File_Table(t *testing.T) {
	testCases := []struct {
		name    string
		file    string
		wantErr error
	}{
		{name: "parent traversal", file: "../up", wantErr: models.ErrEscape},
		{name: "destination itself is not a file", file: "."},
		{name: "under a regular file", file: "plain/child"},
		{name: "over a non-empty directory", file: "full"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g, dest := openGuardWithFile(t)
			require.NoError(t, os.MkdirAll(filepath.Join(dest, "full", "child"), 0o755))

			err := g.File(context.Background(), tc.file, 0o644, strings.NewReader("x"))

			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}
			assert.Error(t, err)
		})
	}
}

func TestGuard_File_ReaderFailureRemovesPartialFile(t *testing.T) {
	g, dest := openGuardWithFile(t)

	err := g.File(context.Background(), "partial", 0o644, failingReader{})

	require.Error(t, err)
	assert.NoFileExists(t, filepath.Join(dest, "partial"))
}

func TestGuard_Symlink_Table(t *testing.T) {
	testCases := []struct {
		name    string
		link    string
		target  string
		wantErr error
	}{
		{name: "parent traversal in name", link: "../up", target: "x", wantErr: models.ErrEscape},
		{name: "target escapes", link: "esc", target: "../../x", wantErr: models.ErrEscape},
		{name: "empty target", link: "empty", target: ""},
		{name: "under a regular file", link: "plain/child", target: "x"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g, _ := openGuardWithFile(t)

			err := g.Symlink(tc.link, tc.target)

			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}
			assert.Error(t, err)
		})
	}
}

func TestGuard_Hardlink_Table(t *testing.T) {
	testCases := []struct {
		name    string
		link    string
		target  string
		wantErr error
		wantOK  bool
	}{
		{name: "links an existing file", link: "copy", target: "plain", wantOK: true},
		{name: "parent traversal in name", link: "../up", target: "plain", wantErr: models.ErrEscape},
		{name: "parent traversal in target", link: "copy", target: "../plain", wantErr: models.ErrEscape},
		{name: "destination itself as target", link: "copy", target: "."},
		{name: "missing target", link: "copy", target: "missing"},
		{name: "target is a symlink", link: "copy", target: "soft"},
		{name: "under a regular file", link: "plain/child", target: "plain"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g, _ := openGuardWithFile(t)
			require.NoError(t, g.Symlink("soft", "plain"))

			err := g.Hardlink(tc.link, tc.target)

			switch {
			case tc.wantOK:
				assert.NoError(t, err)
			case tc.wantErr != nil:
				assert.ErrorIs(t, err, tc.wantErr)
			default:
				assert.Error(t, err)
			}
		})
	}
}

func TestGuard_Copy_ReaderFailureIsReturned(t *testing.T) {
	g, _ := openGuardWithFile(t)

	err := g.Copy(context.Background(), io.Discard, failingReader{})

	assert.Error(t, err)
}

func TestGuard_EntryLimitAppliesToEveryEntryKind(t *testing.T) {
	testCases := []struct {
		name string
		add  func(g *guard.Guard) error
	}{
		{name: "dir", add: func(g *guard.Guard) error { return g.Dir("d", 0o755) }},
		{name: "file", add: func(g *guard.Guard) error {
			return g.File(context.Background(), "f", 0o644, strings.NewReader("x"))
		}},
		{name: "symlink", add: func(g *guard.Guard) error { return g.Symlink("s", "plain") }},
		{name: "hardlink", add: func(g *guard.Guard) error { return g.Hardlink("h", "plain") }},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g, _ := openGuardWithFile(t, guard.LimitEntries(1))

			assert.ErrorIs(t, tc.add(g), models.ErrTooMany)
		})
	}
}

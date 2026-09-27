package shelf

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShelf_PlaceSymlink_Refusals(t *testing.T) {
	f := newFixture(t, "linux")
	wd := f.workdir(t, nsA)
	require.NoError(t, os.MkdirAll(filepath.Join(wd, "dir"), 0o750))
	l, err := f.shelf.layout()
	require.NoError(t, err)
	req := applyRequest{layout: l, bare: bareA, workdir: wd}

	testCases := []struct {
		name   string
		target string
		want   string
	}{
		{name: "missing target", target: filepath.Join(wd, "missing"), want: reasonNotFound},
		{name: "directory target", target: filepath.Join(wd, "dir"), want: reasonWrongType},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := f.shelf.placeSymlink(context.Background(), req, candidate{name: "tool", target: tc.target})
			require.NoError(t, err)
			assert.Equal(t, placement{refused: tc.want}, got)
		})
	}
}

func TestShelf_PlaceSymlink_Errors(t *testing.T) {
	requireUnixHost(t)
	f := newFixture(t, "linux")
	wd := f.workdir(t, nsA)
	target := filepath.Join(wd, "tool")
	writeFile(t, target, "x", 0o755)
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	testCases := []struct {
		name string
		bin  string
	}{
		{name: "location unreadable", bin: file},
		{name: "bin missing", bin: filepath.Join(t.TempDir(), "missing")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := applyRequest{layout: layout{bin: tc.bin, namespaces: f.nsDir}, bare: bareA, workdir: wd}
			_, err := f.shelf.placeSymlink(context.Background(), req, candidate{name: "tool", target: target})
			require.Error(t, err)
		})
	}
}

func TestShelf_PlaceSymlink_SignsOnDarwinARM64(t *testing.T) {
	requireUnixHost(t)
	f := newFixture(t, goosDarwin, WithGOARCH(goarchARM64))
	wd := f.workdir(t, nsA)
	target := filepath.Join(wd, "tool")
	writeFile(t, target, string([]byte{0xcf, 0xfa, 0xed, 0xfe, 0, 0}), 0o755)
	l, err := f.shelf.layout()
	require.NoError(t, err)

	got, err := f.shelf.placeSymlink(context.Background(), applyRequest{layout: l, bare: bareA, workdir: wd}, candidate{name: "tool", target: target})

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(f.bin, "tool"), got.location)
	assert.Equal(t, [][]string{{"codesign", "-v", target}}, f.cmd.calls)
}

func TestShelf_PlaceSymlink_MarksDeclaredTargets(t *testing.T) {
	requireUnixHost(t)
	outsideDir := t.TempDir()

	testCases := []struct {
		name     string
		declared bool
		mode     os.FileMode
		outside  bool
		want     os.FileMode
	}{
		{name: "declared without exec bits gains them", declared: true, mode: 0o644, want: 0o755},
		{name: "declared with an exec bit is left alone", declared: true, mode: 0o740, want: 0o740},
		{name: "auto candidate is never marked", declared: false, mode: 0o644, want: 0o644},
		{name: "declared resolving outside is never marked", declared: true, mode: 0o644, outside: true, want: 0o644},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, "linux")
			wd := f.workdir(t, nsA)
			l, err := f.shelf.layout()
			require.NoError(t, err)
			real := filepath.Join(wd, "tool")
			target := real
			if tc.outside {
				real = filepath.Join(outsideDir, tc.name)
				require.NoError(t, os.Symlink(real, target))
			}
			writeFile(t, real, "x", tc.mode)

			got, err := f.shelf.placeSymlink(context.Background(), applyRequest{layout: l, bare: bareA, workdir: wd}, candidate{name: "tool", target: target, declared: tc.declared})

			require.NoError(t, err)
			assert.Equal(t, filepath.Join(f.bin, "tool"), got.location)
			info, err := os.Stat(real)
			require.NoError(t, err)
			assert.Equal(t, tc.want, info.Mode().Perm())
		})
	}
}

func TestShelf_PlaceSymlink_MarksResolvedPathOnly(t *testing.T) {
	requireUnixHost(t)
	f := newFixture(t, "linux")
	wd := f.workdir(t, nsA)
	l, err := f.shelf.layout()
	require.NoError(t, err)
	real := filepath.Join(wd, "real-tool")
	writeFile(t, real, "x", 0o644)
	target := filepath.Join(wd, "tool")
	require.NoError(t, os.Symlink(real, target))
	resolved, err := filepath.EvalSymlinks(real)
	require.NoError(t, err)
	var chmodded []string
	f.shelf.chmod = func(path string, mode os.FileMode) error {
		chmodded = append(chmodded, path)
		return os.Chmod(path, mode)
	}

	_, err = f.shelf.placeSymlink(context.Background(), applyRequest{layout: l, bare: bareA, workdir: wd}, candidate{name: "tool", target: target, declared: true})

	require.NoError(t, err)
	assert.Equal(t, []string{resolved}, chmodded)
	info, err := os.Lstat(real)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
}

func TestShelf_PlaceSymlink_MarkError(t *testing.T) {
	requireUnixHost(t)
	f := newFixture(t, "linux")
	wd := f.workdir(t, nsA)
	target := filepath.Join(wd, "tool")
	writeFile(t, target, "x", 0o644)
	l, err := f.shelf.layout()
	require.NoError(t, err)
	f.shelf.chmod = func(string, os.FileMode) error { return os.ErrPermission }

	_, err = f.shelf.placeSymlink(context.Background(), applyRequest{layout: l, bare: bareA, workdir: wd}, candidate{name: "tool", target: target, declared: true})

	require.ErrorIs(t, err, os.ErrPermission)
	_, statErr := os.Lstat(filepath.Join(f.bin, "tool"))
	assert.True(t, os.IsNotExist(statErr))
}

func TestShelf_LinkOwner(t *testing.T) {
	f := newFixture(t, goosDarwin)
	bundle := filepath.Join(f.apps[0], "Tool.app")
	require.NoError(t, os.MkdirAll(filepath.Join(bundle, "Contents"), 0o750))
	require.NoError(t, f.tagger.write(bundle, bundleTag(bareA, bundle)))

	testCases := []struct {
		name   string
		goos   string
		target string
		want   string
	}{
		{name: "workdir", goos: "linux", target: filepath.Join(f.nsDir, "github.com", "acme", "tool@v1", "x"), want: string(bareA)},
		{name: "outside on linux", goos: "linux", target: filepath.Join(bundle, "Contents", "tool"), want: ""},
		{name: "tagged bundle on darwin", goos: goosDarwin, target: filepath.Join(bundle, "Contents", "tool"), want: string(bareA)},
		{name: "untagged on darwin", goos: goosDarwin, target: filepath.Join(f.apps[0], "Other.app", "tool"), want: ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f.shelf.goos = tc.goos
			assert.Equal(t, tc.want, string(f.shelf.linkOwner(f.nsDir, tc.target)))
		})
	}
}

func TestShelf_RemoveSymlinks(t *testing.T) {
	requireUnixHost(t)
	f := newFixture(t, "linux")
	own := filepath.Join(f.bin, "own")
	foreign := filepath.Join(f.bin, "foreign")
	plain := filepath.Join(f.bin, "plain")
	require.NoError(t, os.Symlink(filepath.Join(f.nsDir, "github.com", "acme", "tool@v1", "own"), own))
	require.NoError(t, os.Symlink(filepath.Join(f.nsDir, "github.com", "other", "thing@v1", "x"), foreign))
	writeFile(t, plain, "x", 0o755)
	kept := filepath.Join(f.bin, "kept")
	require.NoError(t, os.Symlink(filepath.Join(f.nsDir, "github.com", "acme", "tool@v1", "kept"), kept))
	l, err := f.shelf.layout()
	require.NoError(t, err)

	require.NoError(t, f.shelf.removeSymlinks(l, bareA, map[string]bool{kept: true}))

	assert.NoFileExists(t, own)
	_, err = os.Lstat(kept)
	assert.NoError(t, err)
	_, err = os.Lstat(foreign)
	assert.NoError(t, err)
	assert.FileExists(t, plain)
}

func TestShelf_RemoveSymlinks_Errors(t *testing.T) {
	requireUnixHost(t)
	f := newFixture(t, "linux")
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	assert.NoError(t, f.shelf.removeSymlinks(layout{bin: filepath.Join(t.TempDir(), "missing")}, bareA, nil))
	assert.Error(t, f.shelf.removeSymlinks(layout{bin: file}, bareA, nil))

	if os.Geteuid() == 0 {
		return
	}
	bin := t.TempDir()
	require.NoError(t, os.Symlink(filepath.Join(f.nsDir, "github.com", "acme", "tool@v1", "x"), filepath.Join(bin, "own")))
	require.NoError(t, os.Chmod(bin, 0o500))
	t.Cleanup(func() { _ = os.Chmod(bin, 0o750) })

	assert.Error(t, f.shelf.removeSymlinks(layout{bin: bin, namespaces: f.nsDir}, bareA, nil))
}

package shelf

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandPath(t *testing.T) {
	wd := filepath.Join(t.TempDir(), "wd")

	testCases := []struct {
		name string
		raw  string
		want string
	}{
		{name: "install path", raw: "${INSTALL_PATH}/bin/rg", want: filepath.Join(wd, "bin", "rg")},
		{name: "workdir", raw: "${WORKDIR}/rg", want: filepath.Join(wd, "rg")},
		{name: "relative", raw: "bin/rg", want: filepath.Join(wd, "bin", "rg")},
		{name: "absolute", raw: filepath.Join(wd, "x", "..", "rg"), want: filepath.Join(wd, "rg")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, expandPath(tc.raw, wd))
		})
	}
}

func TestInsideDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "wd")

	testCases := []struct {
		name string
		path string
		want bool
	}{
		{name: "child", path: filepath.Join(dir, "a", "b"), want: true},
		{name: "self", path: dir, want: true},
		{name: "parent", path: filepath.Dir(dir), want: false},
		{name: "sibling", path: dir + "x", want: false},
		{name: "escape", path: filepath.Join(dir, "..", "other"), want: false},
		{name: "relative", path: "rel", want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, insideDir(dir, tc.path))
		})
	}
}

func TestRelInside(t *testing.T) {
	assert.True(t, relInside("a"))
	assert.True(t, relInside("."))
	assert.True(t, relInside("..a"))
	assert.False(t, relInside(".."))
	assert.False(t, relInside(filepath.Join("..", "a")))
}

func TestSafeName(t *testing.T) {
	testCases := []struct {
		name string
		in   string
		want bool
	}{
		{name: "plain", in: "rg", want: true},
		{name: "spaces", in: "My App", want: true},
		{name: "empty", in: "", want: false},
		{name: "dot", in: ".", want: false},
		{name: "dotdot", in: "..", want: false},
		{name: "slash", in: "a/b", want: false},
		{name: "backslash", in: `a\b`, want: false},
		{name: "colon", in: "a:b", want: false},
		{name: "newline", in: "a\nExec=x", want: false},
		{name: "staged suffix", in: "rg" + stagedSuffix, want: false},
		{name: "retired suffix", in: "Tool" + retiredSuffix, want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, safeName(tc.in))
		})
	}
}

func TestContained(t *testing.T) {
	requireUnixHost(t)
	root := t.TempDir()
	wd := filepath.Join(root, "wd")
	outside := filepath.Join(root, "outside")
	writeFile(t, filepath.Join(wd, "real", "tool"), "x", 0o755)
	writeFile(t, filepath.Join(outside, "tool"), "x", 0o755)
	require.NoError(t, os.Symlink(outside, filepath.Join(wd, "escape")))
	require.NoError(t, os.Symlink(filepath.Join(wd, "real"), filepath.Join(wd, "inner")))
	require.NoError(t, os.Symlink(filepath.Join(outside, "tool"), filepath.Join(wd, "link")))
	require.NoError(t, os.Symlink(filepath.Join(wd, "gone"), filepath.Join(wd, "dangling")))
	file := filepath.Join(wd, "file")
	writeFile(t, file, "", 0o600)

	testCases := []struct {
		name    string
		workdir string
		target  string
		want    string
		wantErr bool
	}{
		{name: "plain", workdir: wd, target: filepath.Join(wd, "real", "tool")},
		{name: "internal symlinked parent", workdir: wd, target: filepath.Join(wd, "inner", "tool")},
		{name: "escaping parent", workdir: wd, target: filepath.Join(wd, "escape", "tool"), want: reasonOutsideWorkdir},
		{name: "escaping final link", workdir: wd, target: filepath.Join(wd, "link"), want: reasonOutsideWorkdir},
		{name: "missing parent", workdir: wd, target: filepath.Join(wd, "missing", "tool")},
		{name: "dangling final link", workdir: wd, target: filepath.Join(wd, "dangling")},
		{name: "missing workdir", workdir: filepath.Join(root, "nope"), target: filepath.Join(root, "nope", "x"), wantErr: true},
		{name: "parent is a file", workdir: wd, target: filepath.Join(file, "x"), wantErr: true},
		{name: "target below a file", workdir: wd, target: filepath.Join(wd, "real", "tool", "x"), wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := contained(tc.workdir, tc.target)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestResolvesInside(t *testing.T) {
	requireUnixHost(t)
	wd := t.TempDir()
	outside := filepath.Join(t.TempDir(), "tool")
	writeFile(t, outside, "x", 0o755)
	writeFile(t, filepath.Join(wd, "tool"), "x", 0o755)
	require.NoError(t, os.Symlink(outside, filepath.Join(wd, "link")))

	assert.True(t, resolvesInside(wd, filepath.Join(wd, "tool")))
	assert.False(t, resolvesInside(wd, filepath.Join(wd, "link")))
	assert.False(t, resolvesInside(wd, filepath.Join(wd, "missing")))
	assert.False(t, resolvesInside(filepath.Join(wd, "missing"), filepath.Join(wd, "tool")))
}

func TestResolveInside(t *testing.T) {
	requireUnixHost(t)
	wd, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	outside := filepath.Join(t.TempDir(), "tool")
	writeFile(t, outside, "x", 0o755)
	real := filepath.Join(wd, "tool")
	writeFile(t, real, "x", 0o755)
	require.NoError(t, os.Symlink(real, filepath.Join(wd, "inner")))
	require.NoError(t, os.Symlink(outside, filepath.Join(wd, "outer")))

	testCases := []struct {
		name    string
		workdir string
		target  string
		want    string
		wantOK  bool
	}{
		{name: "regular file", workdir: wd, target: real, want: real, wantOK: true},
		{name: "symlink inside resolves to its target", workdir: wd, target: filepath.Join(wd, "inner"), want: real, wantOK: true},
		{name: "symlink escaping the workdir", workdir: wd, target: filepath.Join(wd, "outer")},
		{name: "missing target", workdir: wd, target: filepath.Join(wd, "missing")},
		{name: "missing workdir", workdir: filepath.Join(wd, "missing"), target: real},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := resolveInside(tc.workdir, tc.target)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestUnsafePath(t *testing.T) {
	testCases := []struct {
		name     string
		path     string
		reserved string
		want     bool
	}{
		{name: "clean", path: "/a/b", reserved: `"%`, want: false},
		{name: "reserved", path: `/a/"b`, reserved: `"%`, want: true},
		{name: "control", path: "/a/\tb", reserved: "", want: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, unsafePath(tc.path, tc.reserved))
		})
	}
}

func TestRequireTarget(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	testCases := []struct {
		name    string
		target  string
		wantDir bool
		want    string
		wantErr bool
	}{
		{name: "file", target: file, wantDir: false, want: ""},
		{name: "dir", target: dir, wantDir: true, want: ""},
		{name: "file wanted dir", target: file, wantDir: true, want: reasonWrongType},
		{name: "dir wanted file", target: dir, wantDir: false, want: reasonWrongType},
		{name: "missing", target: filepath.Join(dir, "missing"), want: reasonNotFound},
		{name: "invalid", target: "bad\x00path", wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := requireTarget(tc.target, tc.wantDir)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

package fsguard

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/workfs"
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
			assert.Equal(t, tc.want, ExpandPath(tc.raw, wd))
		})
	}
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
		{name: "staged suffix", in: "rg" + StagedSuffix, want: false},
		{name: "retired suffix", in: "Tool" + workfs.AsideSuffix, want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, SafeName(tc.in))
		})
	}
}

func TestContained(t *testing.T) {
	mocks.RequireUnixHost(t)
	root := t.TempDir()
	wd := filepath.Join(root, "wd")
	outside := filepath.Join(root, "outside")
	mocks.WriteFile(t, filepath.Join(wd, "real", "tool"), "x", 0o755)
	mocks.WriteFile(t, filepath.Join(outside, "tool"), "x", 0o755)
	require.NoError(t, os.Symlink(outside, filepath.Join(wd, "escape")))
	require.NoError(t, os.Symlink(filepath.Join(wd, "real"), filepath.Join(wd, "inner")))
	require.NoError(t, os.Symlink(filepath.Join(outside, "tool"), filepath.Join(wd, "link")))
	require.NoError(t, os.Symlink(filepath.Join(wd, "gone"), filepath.Join(wd, "dangling")))
	file := filepath.Join(wd, "file")
	mocks.WriteFile(t, file, "", 0o600)

	testCases := []struct {
		name    string
		workdir string
		target  string
		want    string
		wantErr bool
	}{
		{name: "plain", workdir: wd, target: filepath.Join(wd, "real", "tool")},
		{name: "internal symlinked parent", workdir: wd, target: filepath.Join(wd, "inner", "tool")},
		{name: "escaping parent", workdir: wd, target: filepath.Join(wd, "escape", "tool"), want: models.ReasonOutsideWorkdir},
		{name: "escaping final link", workdir: wd, target: filepath.Join(wd, "link"), want: models.ReasonOutsideWorkdir},
		{name: "missing parent", workdir: wd, target: filepath.Join(wd, "missing", "tool")},
		{name: "dangling final link", workdir: wd, target: filepath.Join(wd, "dangling")},
		{name: "missing workdir", workdir: filepath.Join(root, "nope"), target: filepath.Join(root, "nope", "x"), wantErr: true},
		{name: "parent is a file", workdir: wd, target: filepath.Join(file, "x"), wantErr: true},
		{name: "target below a file", workdir: wd, target: filepath.Join(wd, "real", "tool", "x"), wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Contained(tc.workdir, tc.target)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestMarkExecutable(t *testing.T) {
	mocks.RequireUnixHost(t)
	wd := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside")
	mocks.WriteFile(t, outside, "x", 0o644)
	require.NoError(t, os.Symlink(outside, filepath.Join(wd, "link")))
	mocks.WriteFile(t, filepath.Join(wd, "plain"), "x", 0o644)
	mocks.WriteFile(t, filepath.Join(wd, "already"), "x", 0o744)
	require.NoError(t, os.Mkdir(filepath.Join(wd, "dir"), 0o750))

	testCases := []struct {
		name     string
		target   string
		chmodErr error
		want     os.FileMode
		wantErr  error
	}{
		{name: "regular file gains every exec bit", target: "plain", want: 0o755},
		{name: "an exec bit already set is left alone", target: "already", want: 0o744},
		{name: "a directory is left alone", target: "dir", want: 0o750},
		{name: "a target resolving outside is left alone", target: "link", want: 0o644},
		{name: "chmod failure", target: "plain", chmodErr: os.ErrPermission, want: 0o644, wantErr: os.ErrPermission},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(wd, tc.target)
			resolved, err := filepath.EvalSymlinks(path)
			require.NoError(t, err)
			info, err := os.Stat(resolved)
			require.NoError(t, err)
			t.Cleanup(func() { _ = os.Chmod(resolved, info.Mode().Perm()) })
			chmod := os.Chmod
			if tc.chmodErr != nil {
				chmod = func(string, os.FileMode) error { return tc.chmodErr }
			}

			err = MarkExecutable(wd, path, chmod)

			require.ErrorIs(t, err, tc.wantErr)
			got, statErr := os.Stat(resolved)
			require.NoError(t, statErr)
			assert.Equal(t, tc.want, got.Mode().Perm())
		})
	}
}

func TestResolveInside(t *testing.T) {
	mocks.RequireUnixHost(t)
	wd, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	outside := filepath.Join(t.TempDir(), "tool")
	mocks.WriteFile(t, outside, "x", 0o755)
	real := filepath.Join(wd, "tool")
	mocks.WriteFile(t, real, "x", 0o755)
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
			got, ok := ResolveInside(tc.workdir, tc.target)
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
			assert.Equal(t, tc.want, UnsafePath(tc.path, tc.reserved))
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
		{name: "file wanted dir", target: file, wantDir: true, want: models.ReasonWrongType},
		{name: "dir wanted file", target: dir, wantDir: false, want: models.ReasonWrongType},
		{name: "missing", target: filepath.Join(dir, "missing"), want: models.ReasonNotFound},
		{name: "invalid", target: "bad\x00path", wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := RequireTarget(tc.target, tc.wantDir)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

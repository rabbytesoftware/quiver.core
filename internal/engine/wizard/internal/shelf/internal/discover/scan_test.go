package discover

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

func TestWalk_Visit(t *testing.T) {
	root := filepath.Join(t.TempDir(), "root")
	boom := errors.New("boom")

	testCases := []struct {
		name    string
		path    string
		entry   fs.DirEntry
		walkErr error
		opaque  func(string) bool
		want    error
		wantAny bool
	}{
		{name: "walk error", path: root, entry: stubDirEntry{}, walkErr: boom, want: boom},
		{name: "relative path", path: "rel", entry: stubDirEntry{}, wantAny: true},
		{name: "root", path: root, entry: stubDirEntry{mode: fs.ModeDir}},
		{name: "symlink", path: filepath.Join(root, "link"), entry: stubDirEntry{name: "link", mode: fs.ModeSymlink}},
		{name: "info error", path: filepath.Join(root, "f"), entry: stubDirEntry{name: "f", infoErr: boom}, want: boom},
		{name: "deep dir", path: filepath.Join(root, "a", "b", "c", "d"), entry: stubDirEntry{name: "d", mode: fs.ModeDir}, want: filepath.SkipDir},
		{name: "dir above the depth limit", path: filepath.Join(root, "a", "b", "c"), entry: stubDirEntry{name: "c", mode: fs.ModeDir}},
		{name: "shallow dir", path: filepath.Join(root, "a"), entry: stubDirEntry{name: "a", mode: fs.ModeDir}},
		{
			name:   "opaque bundle skipped",
			path:   filepath.Join(root, "Foo.app"),
			entry:  stubDirEntry{name: "Foo.app", mode: fs.ModeDir},
			opaque: IsBundle,
			want:   filepath.SkipDir,
		},
		{
			name:  "bundle descended when not opaque",
			path:  filepath.Join(root, "Foo.app"),
			entry: stubDirEntry{name: "Foo.app", mode: fs.ModeDir},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			w := &walk{scan: Scan{Rule: ExecBits(), Opaque: tc.opaque}, root: root}
			err := w.visit(tc.path, tc.entry, tc.walkErr)
			if tc.wantAny {
				require.Error(t, err)
				return
			}
			assert.ErrorIs(t, err, tc.want)
			assert.Empty(t, w.found)
		})
	}
}

func TestRules(t *testing.T) {
	testCases := []struct {
		name     string
		rule     Rule
		file     string
		mode     fs.FileMode
		wantExec bool
		wantStem string
	}{
		{name: "exec bits set", rule: ExecBits(), file: "tool", mode: 0o700, wantExec: true, wantStem: "tool"},
		{name: "exec bits unset", rule: ExecBits(), file: "tool.exe", mode: 0o644, wantStem: "tool.exe"},
		{name: "exe suffix ignores the mode", rule: ExeSuffix(), file: "Tool.EXE", mode: 0o644, wantExec: true, wantStem: "Tool"},
		{name: "exe suffix rejects other files", rule: ExeSuffix(), file: "tool.bat", mode: 0o755, wantStem: "tool.bat"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.wantExec, tc.rule.Executable(tc.file, tc.mode))
			assert.Equal(t, tc.wantStem, tc.rule.Stem(tc.file))
		})
	}
}

func TestIsHelperFile(t *testing.T) {
	testCases := []struct {
		name string
		file string
		want bool
	}{
		{name: "sandbox", file: "chrome-sandbox", want: true},
		{name: "crashpad", file: "chrome_crashpad_handler", want: true},
		{name: "shared object", file: "libffmpeg.so", want: true},
		{name: "versioned shared object", file: "libssl.so.3", want: true},
		{name: "dylib", file: "libfoo.DYLIB", want: true},
		{name: "app binary", file: "t3", want: false},
		{name: "source lookalike", file: "solver", want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isHelperFile(tc.file))
		})
	}
}

func TestWalk_Descend_SkipsHelperDirs(t *testing.T) {
	w := &walk{scan: Scan{Rule: ExecBits()}}

	for _, dir := range []string{"node_modules", "resources", "lib", "LIB64", "locales"} {
		assert.ErrorIs(t, w.descend(dir, 1), filepath.SkipDir, dir)
	}
	assert.NoError(t, w.descend("bin", 1))
}

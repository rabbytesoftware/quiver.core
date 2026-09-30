package appimage

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDesktopFile_ReadsDesktopEntryGroup(t *testing.T) {
	testCases := []struct {
		name string
		data string
		want desktopEntry
	}{
		{
			name: "name exec and icon",
			data: "[Desktop Entry]\nName=Bruno\nExec=AppRun --no-sandbox %U\nIcon=bruno\nType=Application\n",
			want: desktopEntry{name: "Bruno", exec: "AppRun --no-sandbox %U", icon: "bruno"},
		},
		{
			name: "comments blank lines and spacing",
			data: "# a comment\n\n[Desktop Entry]\n  # indented comment\nName = Spaced App \n\nExec=app\n",
			want: desktopEntry{name: "Spaced App", exec: "app"},
		},
		{
			name: "keys before any group are ignored",
			data: "Name=Orphan\n[Desktop Entry]\nExec=app\n",
			want: desktopEntry{exec: "app"},
		},
		{
			name: "lines without equals are ignored",
			data: "[Desktop Entry]\ngarbage line\nName=App\r\n",
			want: desktopEntry{name: "App"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, parseDesktopFile([]byte(tc.data)))
		})
	}
}

func TestReadCappedDesktopFile_Limits(t *testing.T) {
	head := "[Desktop Entry]\nName=Big\n"
	testCases := []struct {
		name string
		data string
		want desktopEntry
	}{
		{name: "exactly at the cap", data: head + strings.Repeat("#", maxDesktopFileBytes-len(head)), want: desktopEntry{name: "Big"}},
		{name: "over the cap is ignored", data: head + strings.Repeat("#", maxDesktopFileBytes-len(head)+1), want: desktopEntry{}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			entry, err := readCappedDesktopFile(strings.NewReader(tc.data))

			require.NoError(t, err)
			assert.Equal(t, tc.want, entry)
		})
	}
}

func TestReadCappedDesktopFile_ReadError(t *testing.T) {
	errDiskOnFire := errors.New("disk on fire")

	_, err := readCappedDesktopFile(iotest.ErrReader(errDiskOnFire))

	require.ErrorIs(t, err, errDiskOnFire)
}

func TestParseExec_SplitsProgramAndArgs(t *testing.T) {
	testCases := []struct {
		name        string
		line        string
		wantProgram string
		wantArgs    []string
	}{
		{name: "apprun with field code", line: "AppRun --no-sandbox %U", wantProgram: "AppRun", wantArgs: []string{"--no-sandbox"}},
		{name: "quoted program and argument", line: `"bin/app" --flag="a b" %f`, wantProgram: "bin/app", wantArgs: []string{"--flag=a b"}},
		{name: "literal percent", line: "app %% %u", wantProgram: "app", wantArgs: []string{"%"}},
		{name: "absolute program keeps its args for the caller to drop", line: "/usr/bin/app x", wantProgram: "/usr/bin/app", wantArgs: []string{"x"}},
		{name: "escapes inside quotes", line: `app "say \"hi\"" "c:\\dir" "\$HOME" "\` + "`" + `cmd\` + "`" + `"`, wantProgram: "app", wantArgs: []string{`say "hi"`, `c:\dir`, "$HOME", "`cmd`"}},
		{name: "backslash outside quotes is literal", line: `app a\"b`, wantProgram: "app", wantArgs: []string{`a\b`}},
		{name: "every field code is dropped", line: "app %f %F %u %U %d %D %n %N %i %c %k %v %m", wantProgram: "app", wantArgs: []string{}},
		{name: "empty line", line: "", wantProgram: "", wantArgs: nil},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			program, args := parseExec(tc.line)

			assert.Equal(t, tc.wantProgram, program)
			assert.Equal(t, tc.wantArgs, args)
		})
	}
}

func writeAppDir(
	t *testing.T,
	files map[string]string,
	links map[string]string,
) string {
	t.Helper()

	dir := filepath.Join(t.TempDir(), "App.AppDir")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	for name, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}
	for name, target := range links {
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.Symlink(target, path))
	}

	return dir
}

func resolveIconIn(
	t *testing.T,
	dir string,
	name string,
) string {
	t.Helper()

	root, err := os.OpenRoot(dir)
	require.NoError(t, err)
	defer root.Close()

	return resolveIcon(root, name)
}

func TestResolveIcon_Candidates(t *testing.T) {
	hicolor := "usr/share/icons/hicolor/"
	testCases := []struct {
		name  string
		files map[string]string
		links map[string]string
		icon  string
		want  string
	}{
		{
			name:  "root png wins over hicolor",
			files: map[string]string{"app.png": "p", hicolor + "256x256/apps/app.png": "h"},
			icon:  "app",
			want:  "app.png",
		},
		{
			name: "largest square hicolor png",
			files: map[string]string{
				hicolor + "48x48/apps/app.png":    "s",
				hicolor + "512x512/apps/app.png":  "l",
				hicolor + "64x64/apps/other.png":  "o",
				hicolor + "1024x512/apps/app.png": "r",
				hicolor + "axa/apps/app.png":      "n",
				hicolor + "0x0/apps/app.png":      "z",
			},
			icon: "app",
			want: hicolor + "512x512/apps/app.png",
		},
		{
			name:  "scalable svg",
			files: map[string]string{hicolor + "scalable/apps/app.svg": "s"},
			icon:  "app",
			want:  hicolor + "scalable/apps/app.svg",
		},
		{
			name:  "dir icon regular file fallback",
			files: map[string]string{".DirIcon": "d"},
			icon:  "app",
			want:  ".DirIcon",
		},
		{
			name:  "dir icon symlink is ignored",
			files: map[string]string{"real.png": "r"},
			links: map[string]string{".DirIcon": "real.png"},
			icon:  "app",
			want:  "",
		},
		{
			name:  "icon with a slash resolves to nothing",
			files: map[string]string{"sub/app.png": "p", ".DirIcon": "d"},
			icon:  "sub/app",
			want:  "",
		},
		{
			name:  "directory named like the icon is skipped",
			files: map[string]string{"app.png/inner": "x", "app.svg": "s"},
			icon:  "app",
			want:  "app.svg",
		},
		{
			name:  "symlink pointing outside is skipped",
			files: map[string]string{"app.svg": "s"},
			links: map[string]string{"app.png": "../outside.png"},
			icon:  "app",
			want:  "app.svg",
		},
		{
			name:  "symlink pointing inside is followed",
			files: map[string]string{"usr/share/pixmaps/app.png": "p"},
			links: map[string]string{"app.png": "usr/share/pixmaps/app.png"},
			icon:  "app",
			want:  "app.png",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeAppDir(t, tc.files, tc.links)
			require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(dir), "outside.png"), []byte("o"), 0o644))

			assert.Equal(t, tc.want, resolveIconIn(t, dir, tc.icon))
		})
	}
}

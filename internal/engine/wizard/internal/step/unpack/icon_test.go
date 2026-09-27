package unpack

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
			name:  "root svg when no root png",
			files: map[string]string{"app.svg": "s", "app.xpm": "x"},
			icon:  "app",
			want:  "app.svg",
		},
		{
			name:  "root xpm",
			files: map[string]string{"app.xpm": "x"},
			icon:  "app",
			want:  "app.xpm",
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
			name:  "larger size without the icon falls back to a smaller one",
			files: map[string]string{hicolor + "48x48/apps/app.png": "s", hicolor + "512x512/apps/other.png": "o"},
			icon:  "app",
			want:  hicolor + "48x48/apps/app.png",
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
			name:  "empty icon falls back to the dir icon",
			files: map[string]string{".png": "p", ".DirIcon": "d"},
			icon:  "",
			want:  ".DirIcon",
		},
		{
			name:  "empty icon without a dir icon resolves to nothing",
			files: map[string]string{".png": "p"},
			icon:  "",
			want:  "",
		},
		{
			name:  "empty icon ignores a dir icon symlink",
			files: map[string]string{"real.png": "r"},
			links: map[string]string{".DirIcon": "real.png"},
			icon:  "",
			want:  "",
		},
		{
			name:  "icon with a backslash resolves to nothing",
			files: map[string]string{".DirIcon": "d"},
			icon:  `sub\app`,
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
		{
			name:  "nothing found",
			files: map[string]string{"readme": "r"},
			icon:  "app",
			want:  "",
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

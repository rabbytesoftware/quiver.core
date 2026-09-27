package unpack

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadAppImageMeta_FullMetadata(t *testing.T) {
	icon := "usr/share/icons/hicolor/1024x1024/apps/bruno.png"
	dir := writeAppDir(t, map[string]string{
		"AppRun":        "run",
		"bruno.desktop": "[Desktop Entry]\nName=Bruno\nExec=AppRun --no-sandbox %U\nIcon=bruno\n",
		icon:            "png",
	}, map[string]string{".DirIcon": icon})

	meta, err := ReadAppImageMeta(dir)

	require.NoError(t, err)
	assert.Equal(t, AppImageMeta{Name: "Bruno", Args: []string{"--no-sandbox"}, Icon: icon}, meta)
}

func TestReadAppImageMeta_NoDesktopFile(t *testing.T) {
	dir := writeAppDir(t, map[string]string{"AppRun": "run", "bruno.png": "png"}, nil)

	meta, err := ReadAppImageMeta(dir)

	require.NoError(t, err)
	assert.Empty(t, meta.Name)
	assert.Empty(t, meta.Args)
	assert.Empty(t, meta.Icon)
}

func TestReadAppImageMeta_NoDesktopFileFallsBackToDirIcon(t *testing.T) {
	dir := writeAppDir(t, map[string]string{"AppRun": "run", ".DirIcon": "png"}, nil)

	meta, err := ReadAppImageMeta(dir)

	require.NoError(t, err)
	assert.Equal(t, AppImageMeta{Icon: ".DirIcon"}, meta)
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

			meta, err := ReadAppImageMeta(dir)

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

			meta, err := ReadAppImageMeta(dir)

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

	meta, err := ReadAppImageMeta(dir)

	require.NoError(t, err)
	assert.Equal(t, "First", meta.Name)
}

func TestReadAppImageMeta_DesktopFileWithoutName(t *testing.T) {
	dir := writeAppDir(t, map[string]string{"a.desktop": "[Desktop Entry]\nExec=AppRun\n"}, nil)

	meta, err := ReadAppImageMeta(dir)

	require.NoError(t, err)
	assert.Empty(t, meta.Name)
	assert.Empty(t, meta.Args)
}

func TestReadAppImageMeta_MissingAppDir(t *testing.T) {
	_, err := ReadAppImageMeta(filepath.Join(t.TempDir(), "missing"))

	require.Error(t, err)
}

func TestReadAppImageMeta_UnreadableDesktopFile(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permission bits do not block reads here")
	}
	dir := writeAppDir(t, map[string]string{"a.desktop": "[Desktop Entry]\nName=A\n"}, nil)
	require.NoError(t, os.Chmod(filepath.Join(dir, "a.desktop"), 0o000))

	_, err := ReadAppImageMeta(dir)

	require.Error(t, err)
}

package msi

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/mocks"
)

func imageWith(
	t *testing.T,
	files ...string,
) string {
	t.Helper()

	image := t.TempDir()
	for _, name := range files {
		path := filepath.Join(image, filepath.FromSlash(name))
		if strings.HasSuffix(name, "/") {
			require.NoError(t, os.MkdirAll(path, 0o755))
			continue
		}
		mocks.WriteFile(t, path, []byte("x"))
	}

	return image
}

func TestPlanLayout_FindsTheApplicationDirectory(t *testing.T) {
	deep := "PFiles/" + strings.Repeat("d/", maxHoistDepth+2) + "app.exe"

	testCases := []struct {
		name    string
		files   []string
		wantApp string
	}{
		{
			name:    "single root descends through vendor directories",
			files:   []string{"setup.msi", "PFiles/Vendor/App/app.exe", "PFiles/Vendor/App/lib/x.dll"},
			wantApp: "PFiles/Vendor/App",
		},
		{
			name:    "program files root chosen among several roots",
			files:   []string{"Windows/Fonts/x.ttf", "Program Files/App/app.exe", "System64/x.dll"},
			wantApp: "Program Files/App",
		},
		{
			name:    "64-bit folder id",
			files:   []string{"ProgramFiles64Folder/App/app.exe", "CommonFiles64Folder/x.dll"},
			wantApp: "ProgramFiles64Folder/App",
		},
		{
			name:    "root holding several applications is hoisted whole",
			files:   []string{"PFiles/A/a.exe", "PFiles/B/b.exe"},
			wantApp: "PFiles",
		},
		{
			name:    "empty application directory",
			files:   []string{"PFiles/Vendor/"},
			wantApp: "PFiles/Vendor",
		},
		{
			name:  "several unrecognised roots keep the image layout",
			files: []string{"A/a.exe", "B/b.exe"},
		},
		{
			name:  "no directories",
			files: []string{"setup.msi", "app.exe"},
		},
		{
			name:    "descent is capped",
			files:   []string{deep},
			wantApp: "PFiles/" + strings.TrimSuffix(strings.Repeat("d/", maxHoistDepth), "/"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			l, err := planLayout(imageWith(t, tc.files...), "setup.msi")

			require.NoError(t, err)
			assert.Equal(t, layout{copy: "setup.msi", app: filepath.FromSlash(tc.wantApp)}, l)
		})
	}
}

func TestPlanLayout_UnreadableImage(t *testing.T) {
	_, err := planLayout(filepath.Join(t.TempDir(), "missing"), "setup.msi")

	require.ErrorIs(t, err, fs.ErrNotExist)
}

func TestPlanLayout_UnreadableApplicationDirectory(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permission bits do not block reads here")
	}
	image := imageWith(t, "PFiles/App/app.exe")
	locked := filepath.Join(image, "PFiles")
	require.NoError(t, os.Chmod(locked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	_, err := planLayout(image, "setup.msi")

	require.ErrorIs(t, err, fs.ErrPermission)
}

func entryOf(
	t *testing.T,
	dir bool,
) fs.DirEntry {
	t.Helper()

	mode := fs.FileMode(0o644)
	if dir {
		mode = fs.ModeDir | 0o755
	}
	entries, err := fs.ReadDir(fstest.MapFS{"node": {Mode: mode}}, ".")
	require.NoError(t, err)

	return entries[0]
}

func TestLayout_Route(t *testing.T) {
	hoisted := layout{copy: "setup.msi", app: filepath.Join("PFiles", "Vendor", "App")}

	testCases := []struct {
		name     string
		layout   layout
		rel      string
		dir      bool
		wantName string
		wantKeep bool
	}{
		{name: "package copy is dropped", layout: hoisted, rel: "SETUP.MSI"},
		{name: "directory named like the copy is kept", layout: hoisted, rel: "setup.msi", dir: true, wantName: "setup.msi", wantKeep: true},
		{name: "program files root is walked", layout: hoisted, rel: "PFiles", dir: true, wantKeep: true},
		{name: "vendor directory is walked", layout: hoisted, rel: filepath.Join("PFiles", "Vendor"), dir: true, wantKeep: true},
		{name: "application directory is walked", layout: hoisted, rel: hoisted.app, dir: true, wantKeep: true},
		{name: "application file is hoisted", layout: hoisted, rel: filepath.Join(hoisted.app, "app.exe"), wantName: "app.exe", wantKeep: true},
		{name: "nested application file is hoisted", layout: hoisted, rel: filepath.Join(hoisted.app, "lib", "x.dll"), wantName: filepath.Join("lib", "x.dll"), wantKeep: true},
		{name: "other root is kept", layout: hoisted, rel: filepath.Join("System64", "x.dll"), wantName: filepath.Join("System64", "x.dll"), wantKeep: true},
		{name: "sibling with a shared prefix is kept", layout: hoisted, rel: "PFilesX", dir: true, wantName: "PFilesX", wantKeep: true},
		{name: "no hoisting keeps everything", layout: layout{copy: "setup.msi"}, rel: filepath.Join("A", "a.exe"), wantName: filepath.Join("A", "a.exe"), wantKeep: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			name, keep := tc.layout.route("", tc.rel, entryOf(t, tc.dir))

			assert.Equal(t, tc.wantKeep, keep)
			assert.Equal(t, tc.wantName, name)
		})
	}
}

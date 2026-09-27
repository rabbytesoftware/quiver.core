package shelf

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func writeRecord(
	t *testing.T,
	workdir string,
	apps ...domain.PortableApp,
) {
	t.Helper()
	data, err := json.Marshal(domain.PortableRecord{Apps: apps})
	require.NoError(t, err)
	writeFile(t, filepath.Join(workdir, domain.PortableRecordFile), string(data), 0o644)
}

func TestRecord_Candidates_ValidRecord(t *testing.T) {
	requireUnixHost(t)
	wd := t.TempDir()
	writeFile(t, filepath.Join(wd, "Logseq.AppDir", ".quiver-run"), "x", 0o755)
	writeFile(t, filepath.Join(wd, "Logseq.AppDir", "logseq.png"), "x", 0o644)
	writeFile(t, filepath.Join(wd, "tools", "helper"), "x", 0o755)
	writeRecord(t, wd,
		domain.PortableApp{Name: "Logseq", Entry: "Logseq.AppDir/.quiver-run", Icon: "Logseq.AppDir/logseq.png"},
		domain.PortableApp{Name: "Helper", Entry: "tools/./helper"},
	)

	got := recordCandidates(wd, "linux")

	assert.Equal(t, []candidate{
		{name: "Helper", display: "Helper", target: filepath.Join(wd, "tools", "helper"), depth: 1},
		{
			name:    "Logseq",
			display: "Logseq",
			target:  filepath.Join(wd, "Logseq.AppDir", ".quiver-run"),
			icon:    filepath.Join(wd, "Logseq.AppDir", "logseq.png"),
			depth:   1,
		},
	}, got)
}

func TestRecord_Candidates_UnreadableRecord(t *testing.T) {
	testCases := []struct {
		name  string
		setup func(t *testing.T, wd string)
	}{
		{name: "missing file", setup: func(*testing.T, string) {}},
		{
			name: "malformed json",
			setup: func(t *testing.T, wd string) {
				writeFile(t, filepath.Join(wd, domain.PortableRecordFile), "{", 0o644)
			},
		},
		{
			name: "oversize file",
			setup: func(t *testing.T, wd string) {
				writeFile(t, filepath.Join(wd, domain.PortableRecordFile), strings.Repeat(" ", maxRecordBytes+1), 0o644)
			},
		},
		{
			name: "directory",
			setup: func(t *testing.T, wd string) {
				require.NoError(t, os.MkdirAll(filepath.Join(wd, domain.PortableRecordFile), 0o750))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			requireUnixHost(t)
			wd := t.TempDir()
			writeFile(t, filepath.Join(wd, "tool"), "x", 0o755)
			tc.setup(t, wd)

			assert.Nil(t, recordCandidates(wd, "linux"))
		})
	}
}

func TestRecord_Candidates_UnopenableRecord(t *testing.T) {
	requireUnixHost(t)
	if os.Geteuid() == 0 {
		t.Skip("root opens unreadable files")
	}
	wd := t.TempDir()
	writeFile(t, filepath.Join(wd, "tool"), "x", 0o755)
	writeRecord(t, wd, domain.PortableApp{Name: "Tool", Entry: "tool"})
	require.NoError(t, os.Chmod(filepath.Join(wd, domain.PortableRecordFile), 0o000))

	assert.Nil(t, recordCandidates(wd, "linux"))
}

func TestRecord_Candidates_MissingWorkdir(t *testing.T) {
	assert.Nil(t, recordCandidates(filepath.Join(t.TempDir(), "missing"), "linux"))
}

func TestRecord_Candidates_SkipsInvalidEntries(t *testing.T) {
	requireUnixHost(t)
	testCases := []struct {
		name  string
		entry string
	}{
		{name: "parent escape", entry: "../x"},
		{name: "absolute", entry: "/etc/passwd"},
		{name: "empty", entry: ""},
		{name: "missing on disk", entry: "missing"},
		{name: "directory", entry: "dir"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			parent := t.TempDir()
			wd := filepath.Join(parent, "wd")
			writeFile(t, filepath.Join(parent, "x"), "x", 0o755)
			writeFile(t, filepath.Join(wd, "tool"), "x", 0o755)
			require.NoError(t, os.MkdirAll(filepath.Join(wd, "dir"), 0o750))
			writeRecord(t, wd,
				domain.PortableApp{Name: "Bad", Entry: tc.entry},
				domain.PortableApp{Name: "Tool", Entry: "tool"},
			)

			got := recordCandidates(wd, "linux")

			assert.Equal(t, []candidate{{name: "Tool", display: "Tool", target: filepath.Join(wd, "tool"), depth: 1}}, got)
		})
	}
}

func TestRecord_Candidates_SkipsEscapingSymlinks(t *testing.T) {
	requireUnixHost(t)
	wd := t.TempDir()
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "tool"), "x", 0o755)
	writeFile(t, filepath.Join(outside, "icon.png"), "x", 0o644)
	writeFile(t, filepath.Join(wd, "kept"), "x", 0o755)
	require.NoError(t, os.Symlink(filepath.Join(outside, "tool"), filepath.Join(wd, "tool")))
	require.NoError(t, os.Symlink(filepath.Join(outside, "icon.png"), filepath.Join(wd, "icon.png")))
	writeRecord(t, wd,
		domain.PortableApp{Name: "Tool", Entry: "tool"},
		domain.PortableApp{Name: "Kept", Entry: "kept", Icon: "icon.png"},
	)

	got := recordCandidates(wd, "linux")

	assert.Equal(t, []candidate{{name: "Kept", display: "Kept", target: filepath.Join(wd, "kept"), depth: 1}}, got)
}

func TestRecord_Candidates_DropsInvalidIcons(t *testing.T) {
	requireUnixHost(t)
	testCases := []struct {
		name string
		icon string
	}{
		{name: "parent escape", icon: "../icon.png"},
		{name: "absolute", icon: "/etc/icon.png"},
		{name: "missing", icon: "missing.png"},
		{name: "directory", icon: "dir"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			parent := t.TempDir()
			wd := filepath.Join(parent, "wd")
			writeFile(t, filepath.Join(parent, "icon.png"), "x", 0o644)
			writeFile(t, filepath.Join(wd, "tool"), "x", 0o755)
			require.NoError(t, os.MkdirAll(filepath.Join(wd, "dir"), 0o750))
			writeRecord(t, wd, domain.PortableApp{Name: "Tool", Entry: "tool", Icon: tc.icon})

			got := recordCandidates(wd, "linux")

			assert.Equal(t, []candidate{{name: "Tool", display: "Tool", target: filepath.Join(wd, "tool"), depth: 1}}, got)
		})
	}
}

func TestRecord_Candidates_UnsafeNames(t *testing.T) {
	testCases := []struct {
		name        string
		goos        string
		appName     string
		entry       string
		mode        os.FileMode
		wantName    string
		wantDisplay string
		wantSkipped bool
	}{
		{
			name:     "control characters fall back to the entry stem and drop the display",
			goos:     "linux",
			appName:  "Tool\nExec=evil",
			entry:    "tool",
			mode:     0o755,
			wantName: "tool",
		},
		{
			name:        "windows traversal falls back to the exe stem",
			goos:        goosWindows,
			appName:     `..\..\Startup\evil`,
			entry:       "app/OBS.exe",
			mode:        0o644,
			wantName:    "OBS",
			wantDisplay: `..\..\Startup\evil`,
		},
		{
			name:        "slash traversal falls back to the entry stem",
			goos:        "linux",
			appName:     "../../evil",
			entry:       "bin/tool",
			mode:        0o755,
			wantName:    "tool",
			wantDisplay: "../../evil",
		},
		{
			name:        "reserved infix falls back to the entry stem",
			goos:        "linux",
			appName:     "a.quiver-b",
			entry:       "tool",
			mode:        0o755,
			wantName:    "tool",
			wantDisplay: "a.quiver-b",
		},
		{
			name:        "unsafe name and unsafe stem skip the app",
			goos:        "linux",
			appName:     "../evil",
			entry:       "AppDir/.quiver-run",
			mode:        0o755,
			wantSkipped: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.goos != goosWindows {
				requireUnixHost(t)
			}
			wd := t.TempDir()
			writeFile(t, filepath.Join(wd, filepath.FromSlash(tc.entry)), "x", tc.mode)
			writeRecord(t, wd, domain.PortableApp{Name: tc.appName, Entry: tc.entry})

			got := recordCandidates(wd, tc.goos)

			if tc.wantSkipped {
				assert.Empty(t, got)
				return
			}
			assert.Equal(t, []candidate{{
				name:    tc.wantName,
				display: tc.wantDisplay,
				target:  filepath.Join(wd, filepath.FromSlash(tc.entry)),
				depth:   1,
			}}, got)
		})
	}
}

func TestRecord_Candidates_SkipsNonExecutableEntries(t *testing.T) {
	testCases := []struct {
		name  string
		goos  string
		entry string
		mode  os.FileMode
	}{
		{name: "linux without exec bits", goos: "linux", entry: "tool", mode: 0o644},
		{name: "windows without exe suffix", goos: goosWindows, entry: "tool.bat", mode: 0o755},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.goos != goosWindows {
				requireUnixHost(t)
			}
			wd := t.TempDir()
			writeFile(t, filepath.Join(wd, tc.entry), "x", tc.mode)
			writeRecord(t, wd, domain.PortableApp{Name: "Tool", Entry: tc.entry})

			assert.Empty(t, recordCandidates(wd, tc.goos))
		})
	}
}

func TestRecord_Candidates_Bundles(t *testing.T) {
	wd := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(wd, "dist", "Logseq.app", "Contents"), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(wd, "NotABundle"), 0o750))
	writeFile(t, filepath.Join(wd, "File.app"), "x", 0o755)
	require.NoError(t, os.MkdirAll(filepath.Join(wd, "x.quiver-y.app"), 0o750))
	writeRecord(t, wd,
		domain.PortableApp{Name: "Display Name", Entry: "dist/Logseq.app"},
		domain.PortableApp{Name: "Reserved", Entry: "x.quiver-y.app"},
		domain.PortableApp{Name: "NotABundle", Entry: "NotABundle"},
		domain.PortableApp{Name: "File", Entry: "File.app"},
	)

	got := recordCandidates(wd, goosDarwin)

	assert.Equal(t, []candidate{
		{name: "Logseq", display: "Display Name", target: filepath.Join(wd, "dist", "Logseq.app"), depth: 1},
	}, got)
}

func TestRecord_Candidates_BundleSymlinkSkipped(t *testing.T) {
	requireUnixHost(t)
	wd := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(wd, "Real.app"), 0o750))
	require.NoError(t, os.Symlink(filepath.Join(wd, "Real.app"), filepath.Join(wd, "Link.app")))
	writeRecord(t, wd, domain.PortableApp{Name: "Link", Entry: "Link.app"})

	assert.Empty(t, recordCandidates(wd, goosDarwin))
}

func TestRecord_Apply_LinuxDesktopEntryUsesRecordNameAndIcon(t *testing.T) {
	requireUnixHost(t)
	f := newFixture(t, "linux")
	wd := f.workdir(t, nsA)
	writeFile(t, filepath.Join(wd, "Logseq.AppDir", ".quiver-run"), "x", 0o755)
	writeFile(t, filepath.Join(wd, "Logseq.AppDir", "logseq.png"), "x", 0o644)
	writeFile(t, filepath.Join(wd, "Other.AppImage"), "x", 0o755)
	writeRecord(t, wd, domain.PortableApp{Name: "Logseq", Entry: "Logseq.AppDir/.quiver-run", Icon: "Logseq.AppDir/logseq.png"})
	expose := domain.Expose{Desktop: []domain.ExposeEntry{{Name: "logseq", Path: domain.ExposeAuto}}}

	got, err := f.shelf.Apply(context.Background(), nsA, wd, expose, domain.ArrowMedia{Icon: "/media.png"})

	require.NoError(t, err)
	require.Len(t, got.Entries, 1)
	loc := filepath.Join(xdgDir(f.userHome), xdgFileName(bareA, "logseq"))
	assert.Equal(t, loc, got.Entries[0].Location)
	data, err := os.ReadFile(loc)
	require.NoError(t, err)
	target := filepath.Join(wd, "Logseq.AppDir", ".quiver-run")
	icon := filepath.Join(wd, "Logseq.AppDir", "logseq.png")
	assert.Equal(t, xdgContent(bareA, "Logseq", target, icon, []string{}), string(data))
}

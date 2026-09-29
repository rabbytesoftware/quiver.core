package resolve

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/platform"
)

func TestRecord_Candidates_ValidRecord(t *testing.T) {
	mocks.RequireUnixHost(t)
	wd := t.TempDir()
	mocks.WriteFile(t, filepath.Join(wd, "Logseq.AppDir", ".quiver-run"), "x", 0o755)
	mocks.WriteFile(t, filepath.Join(wd, "Logseq.AppDir", "logseq.png"), "x", 0o644)
	mocks.WriteFile(t, filepath.Join(wd, "tools", "helper"), "x", 0o755)
	mocks.WriteRecord(t, wd,
		domain.PortableApp{Name: "Logseq", Entry: "Logseq.AppDir/.quiver-run", Icon: "Logseq.AppDir/logseq.png"},
		domain.PortableApp{Name: "Helper", Entry: "tools/./helper"},
	)

	got := recordCandidates(wd, "linux")

	assert.Equal(t, []models.Candidate{
		{Name: "Helper", Display: "Helper", Target: filepath.Join(wd, "tools", "helper"), Depth: 1},
		{
			Name:    "Logseq",
			Display: "Logseq",
			Target:  filepath.Join(wd, "Logseq.AppDir", ".quiver-run"),
			Icon:    filepath.Join(wd, "Logseq.AppDir", "logseq.png"),
			Depth:   1,
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
				mocks.WriteFile(t, filepath.Join(wd, domain.PortableRecordFile), "{", 0o644)
			},
		},
		{
			name: "oversize file",
			setup: func(t *testing.T, wd string) {
				mocks.WriteFile(t, filepath.Join(wd, domain.PortableRecordFile), strings.Repeat(" ", maxRecordBytes+1), 0o644)
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
			mocks.RequireUnixHost(t)
			wd := t.TempDir()
			mocks.WriteFile(t, filepath.Join(wd, "tool"), "x", 0o755)
			tc.setup(t, wd)

			assert.Nil(t, recordCandidates(wd, "linux"))
		})
	}
}

func TestRecord_Candidates_UnopenableRecord(t *testing.T) {
	mocks.RequireUnixHost(t)
	if os.Geteuid() == 0 {
		t.Skip("root opens unreadable files")
	}
	wd := t.TempDir()
	mocks.WriteFile(t, filepath.Join(wd, "tool"), "x", 0o755)
	mocks.WriteRecord(t, wd, domain.PortableApp{Name: "Tool", Entry: "tool"})
	require.NoError(t, os.Chmod(filepath.Join(wd, domain.PortableRecordFile), 0o000))

	assert.Nil(t, recordCandidates(wd, "linux"))
}

func TestRecord_Candidates_MissingWorkdir(t *testing.T) {
	assert.Nil(t, recordCandidates(filepath.Join(t.TempDir(), "missing"), "linux"))
}

func TestRecord_Candidates_SkipsInvalidEntries(t *testing.T) {
	mocks.RequireUnixHost(t)
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
			mocks.WriteFile(t, filepath.Join(parent, "x"), "x", 0o755)
			mocks.WriteFile(t, filepath.Join(wd, "tool"), "x", 0o755)
			require.NoError(t, os.MkdirAll(filepath.Join(wd, "dir"), 0o750))
			mocks.WriteRecord(t, wd,
				domain.PortableApp{Name: "Bad", Entry: tc.entry},
				domain.PortableApp{Name: "Tool", Entry: "tool"},
			)

			got := recordCandidates(wd, "linux")

			assert.Equal(t, []models.Candidate{{Name: "Tool", Display: "Tool", Target: filepath.Join(wd, "tool"), Depth: 1}}, got)
		})
	}
}

func TestRecord_Candidates_SkipsEscapingSymlinks(t *testing.T) {
	mocks.RequireUnixHost(t)
	wd := t.TempDir()
	outside := t.TempDir()
	mocks.WriteFile(t, filepath.Join(outside, "tool"), "x", 0o755)
	mocks.WriteFile(t, filepath.Join(outside, "icon.png"), "x", 0o644)
	mocks.WriteFile(t, filepath.Join(wd, "kept"), "x", 0o755)
	require.NoError(t, os.Symlink(filepath.Join(outside, "tool"), filepath.Join(wd, "tool")))
	require.NoError(t, os.Symlink(filepath.Join(outside, "icon.png"), filepath.Join(wd, "icon.png")))
	mocks.WriteRecord(t, wd,
		domain.PortableApp{Name: "Tool", Entry: "tool"},
		domain.PortableApp{Name: "Kept", Entry: "kept", Icon: "icon.png"},
	)

	got := recordCandidates(wd, "linux")

	assert.Equal(t, []models.Candidate{{Name: "Kept", Display: "Kept", Target: filepath.Join(wd, "kept"), Depth: 1}}, got)
}

func TestRecord_Candidates_DropsInvalidIcons(t *testing.T) {
	mocks.RequireUnixHost(t)
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
			mocks.WriteFile(t, filepath.Join(parent, "icon.png"), "x", 0o644)
			mocks.WriteFile(t, filepath.Join(wd, "tool"), "x", 0o755)
			require.NoError(t, os.MkdirAll(filepath.Join(wd, "dir"), 0o750))
			mocks.WriteRecord(t, wd, domain.PortableApp{Name: "Tool", Entry: "tool", Icon: tc.icon})

			got := recordCandidates(wd, "linux")

			assert.Equal(t, []models.Candidate{{Name: "Tool", Display: "Tool", Target: filepath.Join(wd, "tool"), Depth: 1}}, got)
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
			goos:        platform.GOOSWindows,
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
			if tc.goos != platform.GOOSWindows {
				mocks.RequireUnixHost(t)
			}
			wd := t.TempDir()
			mocks.WriteFile(t, filepath.Join(wd, filepath.FromSlash(tc.entry)), "x", tc.mode)
			mocks.WriteRecord(t, wd, domain.PortableApp{Name: tc.appName, Entry: tc.entry})

			got := recordCandidates(wd, tc.goos)

			if tc.wantSkipped {
				assert.Empty(t, got)
				return
			}
			assert.Equal(t, []models.Candidate{{
				Name:    tc.wantName,
				Display: tc.wantDisplay,
				Target:  filepath.Join(wd, filepath.FromSlash(tc.entry)),
				Depth:   1,
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
		{name: "windows without exe suffix", goos: platform.GOOSWindows, entry: "tool.bat", mode: 0o755},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.goos != platform.GOOSWindows {
				mocks.RequireUnixHost(t)
			}
			wd := t.TempDir()
			mocks.WriteFile(t, filepath.Join(wd, tc.entry), "x", tc.mode)
			mocks.WriteRecord(t, wd, domain.PortableApp{Name: "Tool", Entry: tc.entry})

			assert.Empty(t, recordCandidates(wd, tc.goos))
		})
	}
}

func TestRecord_Candidates_Bundles(t *testing.T) {
	wd := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(wd, "dist", "Logseq.app", "Contents"), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(wd, "NotABundle"), 0o750))
	mocks.WriteFile(t, filepath.Join(wd, "File.app"), "x", 0o755)
	require.NoError(t, os.MkdirAll(filepath.Join(wd, "x.quiver-y.app"), 0o750))
	mocks.WriteRecord(t, wd,
		domain.PortableApp{Name: "Display Name", Entry: "dist/Logseq.app"},
		domain.PortableApp{Name: "Reserved", Entry: "x.quiver-y.app"},
		domain.PortableApp{Name: "NotABundle", Entry: "NotABundle"},
		domain.PortableApp{Name: "File", Entry: "File.app"},
	)

	got := recordCandidates(wd, platform.GOOSDarwin)

	assert.Equal(t, []models.Candidate{
		{Name: "Logseq", Display: "Display Name", Target: filepath.Join(wd, "dist", "Logseq.app"), Depth: 1},
	}, got)
}

func TestRecord_Candidates_BundleSymlinkSkipped(t *testing.T) {
	mocks.RequireUnixHost(t)
	wd := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(wd, "Real.app"), 0o750))
	require.NoError(t, os.Symlink(filepath.Join(wd, "Real.app"), filepath.Join(wd, "Link.app")))
	mocks.WriteRecord(t, wd, domain.PortableApp{Name: "Link", Entry: "Link.app"})

	assert.Empty(t, recordCandidates(wd, platform.GOOSDarwin))
}

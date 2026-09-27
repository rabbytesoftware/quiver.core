package shelf

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestShelf_Resolve_Declared(t *testing.T) {
	f := newFixture(t, "linux")
	wd := f.workdir(t, nsA)
	req := applyRequest{bare: bareA, workdir: wd}

	testCases := []struct {
		name       string
		entry      domain.ExposeEntry
		want       []candidate
		wantReason string
	}{
		{
			name:  "expanded path",
			entry: domain.ExposeEntry{Name: "rg", Path: "${INSTALL_PATH}/bin/rg"},
			want:  []candidate{{name: "rg", target: filepath.Join(wd, "bin", "rg"), declared: true}},
		},
		{
			name:       "unsafe name",
			entry:      domain.ExposeEntry{Name: "a/b", Path: "rg"},
			wantReason: reasonUnsafeName,
		},
		{
			name:       "outside workdir",
			entry:      domain.ExposeEntry{Name: "rg", Path: "${INSTALL_PATH}/../rg"},
			wantReason: reasonOutsideWorkdir,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, reason, err := f.shelf.resolve(req, domain.ExposeKindCLI, tc.entry)
			require.NoError(t, err)
			assert.Equal(t, tc.wantReason, reason)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestShelf_Resolve_AutoCLI(t *testing.T) {
	requireUnixHost(t)

	testCases := []struct {
		name       string
		files      map[string]os.FileMode
		entryName  string
		want       []string
		wantNames  []string
		wantReason string
	}{
		{
			name:      "single executable keeps its own name over the entry name",
			files:     map[string]os.FileMode{"sub/tool-bin": 0o755, "README": 0o644},
			entryName: "tool",
			want:      []string{"sub/tool-bin"},
			wantNames: []string{"tool-bin"},
		},
		{
			name:      "single executable keeps its name without a safe entry name",
			files:     map[string]os.FileMode{"sub/tool-bin": 0o755},
			want:      []string{"sub/tool-bin"},
			wantNames: []string{"tool-bin"},
		},
		{
			name:      "repo name match",
			files:     map[string]os.FileMode{"bin/tool": 0o755, "bin/helper": 0o755},
			want:      []string{"bin/tool"},
			wantNames: []string{"tool"},
		},
		{
			name:      "entry name match",
			files:     map[string]os.FileMode{"a": 0o755, "b": 0o755},
			entryName: "b",
			want:      []string{"b"},
			wantNames: []string{"b"},
		},
		{
			name:      "shallowest match wins",
			files:     map[string]os.FileMode{"a/tool": 0o755, "tool": 0o755, "z": 0o755},
			want:      []string{"tool"},
			wantNames: []string{"tool"},
		},
		{
			name:      "no match exposes every top-level executable",
			files:     map[string]os.FileMode{"x": 0o755, "y": 0o755, "sub/z": 0o755},
			want:      []string{"x", "y"},
			wantNames: []string{"x", "y"},
		},
		{
			name:       "no match and nothing top-level is ambiguous",
			files:      map[string]os.FileMode{"s1/x": 0o755, "s2/y": 0o755},
			wantReason: reasonAmbiguous,
		},
		{
			name:       "no executable",
			files:      map[string]os.FileMode{"README": 0o644},
			wantReason: reasonNoExecutable,
		},
		{
			name: "skips bundles hidden dirs and deep files",
			files: map[string]os.FileMode{
				"App.app/Contents/MacOS/app": 0o755,
				".git/hooks/pre":             0o755,
				"a/b/c/deep":                 0o755,
				"a/b/ok":                     0o755,
				"bad\tname":                  0o755,
			},
			entryName: "tool",
			want:      []string{"a/b/ok"},
			wantNames: []string{"ok"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, "linux")
			wd := f.workdir(t, nsA)
			for rel, mode := range tc.files {
				writeFile(t, filepath.Join(wd, filepath.FromSlash(rel)), "x", mode)
			}
			req := applyRequest{bare: bareA, workdir: wd}

			got, reason, err := f.shelf.resolve(req, domain.ExposeKindCLI, domain.ExposeEntry{Name: tc.entryName, Path: domain.ExposeAuto})

			require.NoError(t, err)
			assert.Equal(t, tc.wantReason, reason)
			require.Len(t, got, len(tc.want))
			for i, rel := range tc.want {
				assert.Equal(t, filepath.Join(wd, filepath.FromSlash(rel)), got[i].target)
				assert.Equal(t, tc.wantNames[i], got[i].name)
			}
		})
	}
}

func TestShelf_Resolve_AutoCLI_Windows(t *testing.T) {
	f := newFixture(t, goosWindows)
	wd := f.workdir(t, nsA)
	writeFile(t, filepath.Join(wd, "bin", "rg.EXE"), "x", 0o644)
	writeFile(t, filepath.Join(wd, "bin", "notes.txt"), "x", 0o755)
	req := applyRequest{bare: bareA, workdir: wd}

	got, reason, err := f.shelf.resolve(req, domain.ExposeKindCLI, domain.ExposeEntry{Path: domain.ExposeAuto})

	require.NoError(t, err)
	assert.Empty(t, reason)
	assert.Equal(t, []candidate{{name: "rg", target: filepath.Join(wd, "bin", "rg.EXE"), depth: 2}}, got)
}

func TestShelf_Resolve_AutoDesktop(t *testing.T) {
	testCases := []struct {
		name       string
		goos       string
		dirs       []string
		files      []string
		placed     map[string]domain.Namespace
		entryName  string
		want       *candidate
		wantReason string
	}{
		{
			name:      "darwin single bundle keeps its own name",
			goos:      goosDarwin,
			dirs:      []string{"CC Switch.app"},
			entryName: "cc-switch",
			want:      &candidate{name: "CC Switch", target: "CC Switch.app", depth: 1},
		},
		{
			name:      "darwin bundle already moved resolves to the bundle this arrow placed",
			goos:      goosDarwin,
			placed:    map[string]domain.Namespace{"CC Switch.app": bareA, "Other.app": bareB},
			entryName: "cc-switch",
			want:      &candidate{name: "CC Switch", target: "CC Switch.app", depth: 1},
		},
		{
			name:       "darwin bundle placed by another arrow is not ours",
			goos:       goosDarwin,
			placed:     map[string]domain.Namespace{"Other.app": bareB},
			entryName:  "Tool",
			wantReason: reasonNoDesktop,
		},
		{
			name:       "darwin nothing and no name",
			goos:       goosDarwin,
			wantReason: reasonNoDesktop,
		},
		{
			name: "darwin several bundles pick the repo name",
			goos: goosDarwin,
			dirs: []string{"Other.app", "Tool.app"},
			want: &candidate{name: "Tool", target: "Tool.app", depth: 1},
		},
		{
			name:       "darwin several bundles without a match",
			goos:       goosDarwin,
			dirs:       []string{"One.app", "Two.app"},
			wantReason: reasonAmbiguous,
		},
		{
			name:      "linux appimage",
			goos:      "linux",
			dirs:      []string{"dir.AppImage"},
			files:     []string{"Tool-x86_64.AppImage", "notes.txt"},
			entryName: "Tool",
			want:      &candidate{name: "Tool", target: "Tool-x86_64.AppImage", depth: 1},
		},
		{
			name:      "linux several appimages pick the entry name",
			goos:      "linux",
			files:     []string{"A.AppImage", "B.AppImage"},
			entryName: "B",
			want:      &candidate{name: "B", target: "B.AppImage", depth: 1},
		},
		{
			name:       "linux ignores bundles",
			goos:       "linux",
			dirs:       []string{"Tool.app"},
			entryName:  "Tool",
			wantReason: reasonNoDesktop,
		},
		{
			name:  "windows single exe",
			goos:  goosWindows,
			files: []string{"setup.exe"},
			want:  &candidate{name: "setup", target: "setup.exe", depth: 1},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, tc.goos)
			wd := f.workdir(t, nsA)
			for _, d := range tc.dirs {
				require.NoError(t, os.MkdirAll(filepath.Join(wd, d), 0o750))
			}
			for _, file := range tc.files {
				writeFile(t, filepath.Join(wd, file), "x", 0o755)
			}
			for bundle, owner := range tc.placed {
				path := filepath.Join(f.apps[0], bundle)
				require.NoError(t, os.MkdirAll(path, 0o750))
				require.NoError(t, f.tagger.write(path, bundleTag(owner, path)))
			}
			writeFile(t, filepath.Join(f.apps[0], "Stray.app"), "x", 0o644)
			req := applyRequest{bare: bareA, workdir: wd, layout: layout{apps: f.apps}}

			got, reason, err := f.shelf.resolve(req, domain.ExposeKindDesktop, domain.ExposeEntry{Name: tc.entryName, Path: domain.ExposeAuto})

			require.NoError(t, err)
			assert.Equal(t, tc.wantReason, reason)
			if tc.want == nil {
				assert.Empty(t, got)
				return
			}
			want := *tc.want
			want.target = filepath.Join(wd, want.target)
			assert.Equal(t, []candidate{want}, got)
		})
	}
}

func TestShelf_Resolve_AutoDesktop_IgnoresSymlinks(t *testing.T) {
	requireUnixHost(t)

	testCases := []struct {
		name string
		goos string
		link string
		dir  bool
	}{
		{name: "linux appimage", goos: "linux", link: "X.AppImage"},
		{name: "windows exe", goos: goosWindows, link: "x.exe"},
		{name: "darwin bundle", goos: goosDarwin, link: "X.app", dir: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, tc.goos)
			wd := f.workdir(t, nsA)
			outside := filepath.Join(t.TempDir(), tc.link)
			if tc.dir {
				require.NoError(t, os.MkdirAll(outside, 0o750))
			}
			if !tc.dir {
				writeFile(t, outside, "x", 0o755)
			}
			require.NoError(t, os.Symlink(outside, filepath.Join(wd, tc.link)))

			got, reason, err := f.shelf.resolve(applyRequest{bare: bareA, workdir: wd}, domain.ExposeKindDesktop, domain.ExposeEntry{Path: domain.ExposeAuto})

			require.NoError(t, err)
			assert.Empty(t, got)
			assert.Equal(t, reasonNoDesktop, reason)
		})
	}
}

func TestShelf_Resolve_AutoDesktop_Record(t *testing.T) {
	testCases := []struct {
		name       string
		goos       string
		apps       []domain.PortableApp
		dirs       []string
		files      []string
		entryName  string
		want       *candidate
		wantReason string
	}{
		{
			name:      "record app wins over a top-level appimage",
			goos:      "linux",
			apps:      []domain.PortableApp{{Name: "Logseq", Entry: "Logseq.AppDir/.quiver-run", Icon: "Logseq.AppDir/logseq.png"}},
			files:     []string{"Logseq.AppDir/.quiver-run", "Logseq.AppDir/logseq.png", "Tool.AppImage"},
			entryName: "logseq",
			want: &candidate{
				name:    "logseq",
				display: "Logseq",
				target:  "Logseq.AppDir/.quiver-run",
				icon:    "Logseq.AppDir/logseq.png",
				depth:   1,
			},
		},
		{
			name: "several record apps pick the repo name",
			goos: "linux",
			apps: []domain.PortableApp{
				{Name: "Helper", Entry: "helper/run"},
				{Name: "Tool", Entry: "tool/run"},
			},
			files:     []string{"helper/run", "tool/run"},
			entryName: "app",
			want:      &candidate{name: "app", display: "Tool", target: "tool/run", depth: 1},
		},
		{
			name: "several record apps pick the entry name",
			goos: goosWindows,
			apps: []domain.PortableApp{
				{Name: "Helper", Entry: "helper.exe"},
				{Name: "Studio", Entry: "studio.exe"},
			},
			files:     []string{"helper.exe", "studio.exe"},
			entryName: "Studio",
			want:      &candidate{name: "Studio", display: "Studio", target: "studio.exe", depth: 1},
		},
		{
			name: "same-name record apps pick the most recently recorded",
			goos: "linux",
			apps: []domain.PortableApp{
				{Name: "Bruno", Entry: "Bruno-1.0/.quiver-run"},
				{Name: "Bruno", Entry: "Bruno-2.0/.quiver-run"},
			},
			files:     []string{"Bruno-1.0/.quiver-run", "Bruno-2.0/.quiver-run"},
			entryName: "bruno",
			want:      &candidate{name: "bruno", display: "Bruno", target: "Bruno-2.0/.quiver-run", depth: 1},
		},
		{
			name: "several record apps without a match are ambiguous",
			goos: "linux",
			apps: []domain.PortableApp{
				{Name: "One", Entry: "one"},
				{Name: "Two", Entry: "two"},
			},
			files:      []string{"one", "two"},
			entryName:  "app",
			wantReason: reasonAmbiguous,
		},
		{
			name:      "darwin record bundle keeps its own name",
			goos:      goosDarwin,
			apps:      []domain.PortableApp{{Name: "Logseq App", Entry: "dist/Logseq.app"}},
			dirs:      []string{"dist/Logseq.app", "Other.app"},
			entryName: "logseq",
			want:      &candidate{name: "Logseq", display: "Logseq App", target: "dist/Logseq.app", depth: 1},
		},
		{
			name:      "record without a usable app falls back to native candidates",
			goos:      "linux",
			apps:      []domain.PortableApp{{Name: "Gone", Entry: "gone"}},
			files:     []string{"Tool.AppImage"},
			entryName: "tool",
			want:      &candidate{name: "tool", target: "Tool.AppImage", depth: 1},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.goos != goosWindows {
				requireUnixHost(t)
			}
			f := newFixture(t, tc.goos)
			wd := f.workdir(t, nsA)
			for _, d := range tc.dirs {
				require.NoError(t, os.MkdirAll(filepath.Join(wd, filepath.FromSlash(d)), 0o750))
			}
			for _, file := range tc.files {
				writeFile(t, filepath.Join(wd, filepath.FromSlash(file)), "x", 0o755)
			}
			writeRecord(t, wd, tc.apps...)
			req := applyRequest{bare: bareA, workdir: wd, layout: layout{apps: f.apps}}

			got, reason, err := f.shelf.resolve(req, domain.ExposeKindDesktop, domain.ExposeEntry{Name: tc.entryName, Path: domain.ExposeAuto})

			require.NoError(t, err)
			assert.Equal(t, tc.wantReason, reason)
			if tc.want == nil {
				assert.Empty(t, got)
				return
			}
			want := *tc.want
			want.target = filepath.Join(wd, filepath.FromSlash(want.target))
			if want.icon != "" {
				want.icon = filepath.Join(wd, filepath.FromSlash(want.icon))
			}
			assert.Equal(t, []candidate{want}, got)
		})
	}
}

func TestShelf_Resolve_AutoDesktop_ScanFallback(t *testing.T) {
	testCases := []struct {
		name       string
		goos       string
		bare       domain.Namespace
		files      map[string]os.FileMode
		entryName  string
		want       *candidate
		wantReason string
	}{
		{
			name:      "linux archive tree resolves the executable named after the repo",
			goos:      "linux",
			bare:      "github.com/logseq/logseq",
			files:     map[string]os.FileMode{"logseq/Logseq": 0o755, "logseq/chrome-sandbox": 0o755},
			entryName: "logseq-desktop",
			want:      &candidate{name: "logseq-desktop", target: "logseq/Logseq", depth: 2},
		},
		{
			name:      "linux archive tree resolves the executable named after the entry",
			goos:      "linux",
			bare:      "github.com/acme/suite",
			files:     map[string]os.FileMode{"suite-1.0/editor": 0o755, "suite-1.0/viewer": 0o755},
			entryName: "viewer",
			want:      &candidate{name: "viewer", target: "suite-1.0/viewer", depth: 2},
		},
		{
			name:      "windows archive tree resolves the exe named after the repo",
			goos:      goosWindows,
			bare:      "github.com/obsproject/obs",
			files:     map[string]os.FileMode{"app/OBS.exe": 0o644, "app/helper.exe": 0o644},
			entryName: "obs-studio",
			want:      &candidate{name: "obs-studio", target: "app/OBS.exe", depth: 2},
		},
		{
			name:       "linux executables without a matching name are not desktop apps",
			goos:       "linux",
			bare:       "github.com/logseq/logseq",
			files:      map[string]os.FileMode{"bin/helper": 0o755},
			entryName:  "logseq-desktop",
			wantReason: reasonNoDesktop,
		},
		{
			name:       "darwin gets no scan fallback",
			goos:       goosDarwin,
			bare:       "github.com/logseq/logseq",
			files:      map[string]os.FileMode{"logseq/Logseq": 0o755},
			entryName:  "logseq",
			wantReason: reasonNoDesktop,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.goos != goosWindows {
				requireUnixHost(t)
			}
			f := newFixture(t, tc.goos)
			wd := f.workdir(t, nsA)
			for rel, mode := range tc.files {
				writeFile(t, filepath.Join(wd, filepath.FromSlash(rel)), "x", mode)
			}
			req := applyRequest{bare: tc.bare, workdir: wd, layout: layout{apps: f.apps}}

			got, reason, err := f.shelf.resolve(req, domain.ExposeKindDesktop, domain.ExposeEntry{Name: tc.entryName, Path: domain.ExposeAuto})

			require.NoError(t, err)
			assert.Equal(t, tc.wantReason, reason)
			if tc.want == nil {
				assert.Empty(t, got)
				return
			}
			want := *tc.want
			want.target = filepath.Join(wd, filepath.FromSlash(want.target))
			assert.Equal(t, []candidate{want}, got)
		})
	}
}

func TestShelf_Resolve_AutoDesktop_ScanError(t *testing.T) {
	requireUnixHost(t)
	if os.Geteuid() == 0 {
		t.Skip("root reads unreadable directories")
	}
	f := newFixture(t, "linux")
	wd := f.workdir(t, nsA)
	locked := filepath.Join(wd, "locked")
	require.NoError(t, os.MkdirAll(locked, 0o750))
	require.NoError(t, os.Chmod(locked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o750) })

	_, _, err := f.shelf.resolve(applyRequest{bare: bareA, workdir: wd}, domain.ExposeKindDesktop, domain.ExposeEntry{Name: "tool", Path: domain.ExposeAuto})

	require.Error(t, err)
}

func TestShelf_Resolve_Declared_ContainmentError(t *testing.T) {
	f := newFixture(t, "linux")
	req := applyRequest{bare: bareA, workdir: filepath.Join(t.TempDir(), "missing")}

	_, _, err := f.shelf.resolve(req, domain.ExposeKindCLI, domain.ExposeEntry{Name: "rg", Path: "rg"})

	require.Error(t, err)
}

func TestShelf_Resolve_AutoDesktop_MissingWorkdir(t *testing.T) {
	f := newFixture(t, "linux")
	req := applyRequest{bare: bareA, workdir: filepath.Join(t.TempDir(), "missing")}

	_, _, err := f.shelf.resolve(req, domain.ExposeKindDesktop, domain.ExposeEntry{Name: "x", Path: domain.ExposeAuto})

	require.Error(t, err)
}

func TestRepoName(t *testing.T) {
	assert.Equal(t, "tool", repoName(bareA))
	assert.Equal(t, "r", repoName("q.io/u/r/auid"))
	assert.Empty(t, repoName("short/ns"))
}

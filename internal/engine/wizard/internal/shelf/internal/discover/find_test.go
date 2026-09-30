package discover

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/mocks"
)

func unixScan() Scan {
	return Scan{Rule: ExecBits()}
}

func darwinScan() Scan {
	return Scan{Rule: ExecBits(), Opaque: IsBundle}
}

func windowsScan() Scan {
	return Scan{Rule: ExeSuffix()}
}

func TestDeclared(t *testing.T) {
	sb := mocks.NewSandbox(t, "linux")
	wd := sb.Workdir(t, mocks.NsA)
	req := models.Request{Bare: mocks.BareA, Workdir: wd}

	testCases := []struct {
		name       string
		entry      domain.ExposeEntry
		want       []models.Candidate
		wantReason string
	}{
		{
			name:  "expanded path",
			entry: domain.ExposeEntry{Name: "rg", Path: "${INSTALL_PATH}/bin/rg"},
			want:  []models.Candidate{{Name: "rg", Target: filepath.Join(wd, "bin", "rg"), Declared: true}},
		},
		{
			name:       "unsafe name",
			entry:      domain.ExposeEntry{Name: "a/b", Path: "rg"},
			wantReason: models.ReasonUnsafeName,
		},
		{
			name:       "outside workdir",
			entry:      domain.ExposeEntry{Name: "rg", Path: "${INSTALL_PATH}/../rg"},
			wantReason: models.ReasonOutsideWorkdir,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, reason, err := Declared(req, tc.entry)
			require.NoError(t, err)
			assert.Equal(t, tc.wantReason, reason)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestDeclared_ContainmentError(t *testing.T) {
	req := models.Request{Bare: mocks.BareA, Workdir: filepath.Join(t.TempDir(), "missing")}

	_, _, err := Declared(req, domain.ExposeEntry{Name: "rg", Path: "rg"})

	require.Error(t, err)
}

func TestCommands(t *testing.T) {
	mocks.RequireUnixHost(t)

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
			name:      "no match exposes every executable at the shallowest depth",
			files:     map[string]os.FileMode{"s1/x": 0o755, "s2/y": 0o755, "s1/deep/z": 0o755},
			want:      []string{"s1/x", "s2/y"},
			wantNames: []string{"x", "y"},
		},
		{
			name:      "one directory deeper keeps every executable",
			files:     map[string]os.FileMode{"tool/foo": 0o755, "tool/bar": 0o755},
			want:      []string{"tool/bar", "tool/foo"},
			wantNames: []string{"bar", "foo"},
		},
		{
			name:      "binary nested four levels deep is found",
			files:     map[string]os.FileMode{"tool/tool-v1-linux/bin/tool": 0o755, "tool/tool-v1-linux/bin/helper": 0o755},
			entryName: "tool",
			want:      []string{"tool/tool-v1-linux/bin/tool"},
			wantNames: []string{"tool"},
		},
		{
			name:       "no executable",
			files:      map[string]os.FileMode{"README": 0o644},
			wantReason: models.ReasonNoExecutable,
		},
		{
			name: "skips hidden dirs and deep files",
			files: map[string]os.FileMode{
				".git/hooks/pre": 0o755,
				"a/b/c/d/deep":   0o755,
				"a/b/ok":         0o755,
				"bad\tname":      0o755,
			},
			entryName: "tool",
			want:      []string{"a/b/ok"},
			wantNames: []string{"ok"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sb := mocks.NewSandbox(t, "linux")
			wd := sb.Workdir(t, mocks.NsA)
			for rel, mode := range tc.files {
				mocks.WriteFile(t, filepath.Join(wd, filepath.FromSlash(rel)), "x", mode)
			}
			req := models.Request{Bare: mocks.BareA, Workdir: wd}

			got, reason, err := Commands(req, domain.ExposeEntry{Name: tc.entryName, Path: domain.ExposeAuto}, unixScan())

			require.NoError(t, err)
			assert.Equal(t, tc.wantReason, reason)
			require.Len(t, got, len(tc.want))
			for i, rel := range tc.want {
				assert.Equal(t, filepath.Join(wd, filepath.FromSlash(rel)), got[i].Target)
				assert.Equal(t, tc.wantNames[i], got[i].Name)
			}
		})
	}
}

func TestCommands_AppSuffixedDirectory(t *testing.T) {
	mocks.RequireUnixHost(t)

	testCases := []struct {
		name       string
		scan       Scan
		want       string
		wantReason string
	}{
		{name: "linux descends into it", scan: unixScan(), want: filepath.Join("zed.app", "bin", "zed")},
		{name: "darwin never looks inside a bundle", scan: darwinScan(), wantReason: models.ReasonNoExecutable},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sb := mocks.NewSandbox(t, "linux")
			wd := sb.Workdir(t, mocks.NsA)
			mocks.WriteFile(t, filepath.Join(wd, "zed.app", "bin", "zed"), "x", 0o755)
			mocks.WriteFile(t, filepath.Join(wd, "zed.app", "libexec", "zed-editor"), "x", 0o755)
			req := models.Request{Bare: "github.com/zed-industries/zed", Workdir: wd}

			got, reason, err := Commands(req, domain.ExposeEntry{Name: "zed", Path: domain.ExposeAuto}, tc.scan)

			require.NoError(t, err)
			assert.Equal(t, tc.wantReason, reason)
			if tc.want == "" {
				assert.Empty(t, got)
				return
			}
			assert.Equal(t, []models.Candidate{{Name: "zed", Target: filepath.Join(wd, tc.want), Depth: 3}}, got)
		})
	}
}

func TestCommands_Windows(t *testing.T) {
	sb := mocks.NewSandbox(t, "windows")
	wd := sb.Workdir(t, mocks.NsA)
	mocks.WriteFile(t, filepath.Join(wd, "bin", "rg.EXE"), "x", 0o644)
	mocks.WriteFile(t, filepath.Join(wd, "bin", "notes.txt"), "x", 0o755)
	req := models.Request{Bare: mocks.BareA, Workdir: wd}

	got, reason, err := Commands(req, domain.ExposeEntry{Path: domain.ExposeAuto}, windowsScan())

	require.NoError(t, err)
	assert.Empty(t, reason)
	assert.Equal(t, []models.Candidate{{Name: "rg", Target: filepath.Join(wd, "bin", "rg.EXE"), Depth: 2}}, got)
}

func TestCommands_ScanError(t *testing.T) {
	req := models.Request{Bare: mocks.BareA, Workdir: filepath.Join(t.TempDir(), "missing")}

	_, _, err := Commands(req, domain.ExposeEntry{Path: domain.ExposeAuto}, unixScan())

	require.Error(t, err)
}

func TestLaunchers(t *testing.T) {
	testCases := []struct {
		name       string
		scan       Scan
		suffix     string
		bare       domain.Namespace
		dirs       []string
		files      map[string]os.FileMode
		entryName  string
		want       *models.Candidate
		wantReason string
	}{
		{
			name:      "linux appimage takes the entry name",
			scan:      unixScan(),
			suffix:    models.AppImageExt,
			dirs:      []string{"dir.AppImage"},
			files:     map[string]os.FileMode{"Tool-x86_64.AppImage": 0o755, "notes.txt": 0o755},
			entryName: "Tool",
			want:      &models.Candidate{Name: "Tool", Target: "Tool-x86_64.AppImage", Depth: 1},
		},
		{
			name:      "linux several appimages pick the entry name",
			scan:      unixScan(),
			suffix:    models.AppImageExt,
			files:     map[string]os.FileMode{"A.AppImage": 0o755, "B.AppImage": 0o755},
			entryName: "B",
			want:      &models.Candidate{Name: "B", Target: "B.AppImage", Depth: 1},
		},
		{
			name:       "linux several appimages without a match",
			scan:       unixScan(),
			suffix:     models.AppImageExt,
			files:      map[string]os.FileMode{"A.AppImage": 0o755, "B.AppImage": 0o755},
			wantReason: models.ReasonAmbiguous,
		},
		{
			name:       "linux ignores bundles",
			scan:       unixScan(),
			suffix:     models.AppImageExt,
			dirs:       []string{"Tool.app"},
			entryName:  "Tool",
			wantReason: models.ReasonNoDesktop,
		},
		{
			name:   "windows single exe",
			scan:   windowsScan(),
			suffix: models.ExeExt,
			files:  map[string]os.FileMode{"setup.exe": 0o644},
			want:   &models.Candidate{Name: "setup", Target: "setup.exe", Depth: 1},
		},
		{
			name:      "linux archive tree resolves the executable named after the repo",
			scan:      unixScan(),
			suffix:    models.AppImageExt,
			bare:      "github.com/logseq/logseq",
			files:     map[string]os.FileMode{"logseq/Logseq": 0o755, "logseq/chrome-sandbox": 0o755},
			entryName: "logseq-desktop",
			want:      &models.Candidate{Name: "logseq-desktop", Target: "logseq/Logseq", Depth: 2},
		},
		{
			name:      "linux archive tree resolves the executable named after the entry",
			scan:      unixScan(),
			suffix:    models.AppImageExt,
			bare:      "github.com/acme/suite",
			files:     map[string]os.FileMode{"suite-1.0/editor": 0o755, "suite-1.0/viewer": 0o755},
			entryName: "viewer",
			want:      &models.Candidate{Name: "viewer", Target: "suite-1.0/viewer", Depth: 2},
		},
		{
			name:      "windows archive tree resolves the exe named after the repo",
			scan:      windowsScan(),
			suffix:    models.ExeExt,
			bare:      "github.com/obsproject/obs",
			files:     map[string]os.FileMode{"app/OBS.exe": 0o644, "app/helper.exe": 0o644},
			entryName: "obs-studio",
			want:      &models.Candidate{Name: "obs-studio", Target: "app/OBS.exe", Depth: 2},
		},
		{
			name:      "linux single unmatched executable is the desktop app",
			scan:      unixScan(),
			suffix:    models.AppImageExt,
			bare:      "github.com/logseq/logseq",
			files:     map[string]os.FileMode{"bin/helper": 0o755},
			entryName: "logseq-desktop",
			want:      &models.Candidate{Name: "logseq-desktop", Target: "bin/helper", Depth: 2},
		},
		{
			name:       "linux several unmatched executables are not desktop apps",
			scan:       unixScan(),
			suffix:     models.AppImageExt,
			bare:       "github.com/logseq/logseq",
			files:      map[string]os.FileMode{"bin/helper": 0o755, "bin/other": 0o755},
			entryName:  "logseq-desktop",
			wantReason: models.ReasonNoDesktop,
		},
		{
			name:      "linux helpers do not hide the single app",
			scan:      unixScan(),
			suffix:    models.AppImageExt,
			bare:      "github.com/pingdotgg/t3code",
			files:     map[string]os.FileMode{"t3/t3": 0o755, "t3/chrome-sandbox": 0o755, "t3/libffmpeg.so": 0o755, "t3/lib/x": 0o755},
			entryName: "t3code",
			want:      &models.Candidate{Name: "t3code", Target: "t3/t3", Depth: 2},
		},
		{
			name:      "windows single unmatched exe inside a wrapper directory",
			scan:      windowsScan(),
			suffix:    models.ExeExt,
			bare:      "github.com/pingdotgg/t3code",
			files:     map[string]os.FileMode{"t3-0.0.44-win32-x64/t3.exe": 0o644, "t3-0.0.44-win32-x64/resources/elevate.exe": 0o644},
			entryName: "t3code",
			want:      &models.Candidate{Name: "t3code", Target: "t3-0.0.44-win32-x64/t3.exe", Depth: 2},
		},
		{
			name:      "linux app suffixed archive tree is scanned like any directory",
			scan:      unixScan(),
			suffix:    models.AppImageExt,
			bare:      "github.com/zed-industries/zed",
			files:     map[string]os.FileMode{"zed.app/bin/zed": 0o755, "zed.app/libexec/zed-editor": 0o755},
			entryName: "zed",
			want:      &models.Candidate{Name: "zed", Target: "zed.app/bin/zed", Depth: 3},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.suffix != models.ExeExt {
				mocks.RequireUnixHost(t)
			}
			sb := mocks.NewSandbox(t, "linux")
			wd := sb.Workdir(t, mocks.NsA)
			for _, d := range tc.dirs {
				require.NoError(t, os.MkdirAll(filepath.Join(wd, d), 0o750))
			}
			for rel, mode := range tc.files {
				mocks.WriteFile(t, filepath.Join(wd, filepath.FromSlash(rel)), "x", mode)
			}
			bare := tc.bare
			if bare == "" {
				bare = mocks.BareA
			}
			req := models.Request{Bare: bare, Workdir: wd}

			got, reason, err := Launchers(req, domain.ExposeEntry{Name: tc.entryName, Path: domain.ExposeAuto}, tc.scan, tc.suffix)

			require.NoError(t, err)
			assert.Equal(t, tc.wantReason, reason)
			if tc.want == nil {
				assert.Empty(t, got)
				return
			}
			want := *tc.want
			want.Target = filepath.Join(wd, filepath.FromSlash(want.Target))
			assert.Equal(t, []models.Candidate{want}, got)
		})
	}
}

func TestLaunchers_PrefersTheRecord(t *testing.T) {
	mocks.RequireUnixHost(t)
	sb := mocks.NewSandbox(t, "linux")
	wd := sb.Workdir(t, mocks.NsA)
	mocks.WriteFile(t, filepath.Join(wd, "Other.AppImage"), "x", 0o755)
	mocks.WriteFile(t, filepath.Join(wd, "App", "run"), "x", 0o755)
	mocks.WriteRecord(t, wd, domain.PortableApp{Name: "Tool App", Entry: "App/run"})
	req := models.Request{Bare: mocks.BareA, Workdir: wd}

	got, reason, err := Launchers(req, domain.ExposeEntry{Name: "tool", Path: domain.ExposeAuto}, unixScan(), models.AppImageExt)

	require.NoError(t, err)
	assert.Empty(t, reason)
	assert.Equal(t, []models.Candidate{{Name: "tool", Display: "Tool App", Target: filepath.Join(wd, "App", "run"), Depth: 1}}, got)
}

func TestLaunchers_IgnoresSymlinks(t *testing.T) {
	mocks.RequireUnixHost(t)

	testCases := []struct {
		name   string
		scan   Scan
		suffix string
		link   string
	}{
		{name: "linux appimage", scan: unixScan(), suffix: models.AppImageExt, link: "X.AppImage"},
		{name: "windows exe", scan: windowsScan(), suffix: models.ExeExt, link: "x.exe"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sb := mocks.NewSandbox(t, "linux")
			wd := sb.Workdir(t, mocks.NsA)
			outside := filepath.Join(t.TempDir(), tc.link)
			mocks.WriteFile(t, outside, "x", 0o755)
			require.NoError(t, os.Symlink(outside, filepath.Join(wd, tc.link)))

			got, reason, err := Launchers(models.Request{Bare: mocks.BareA, Workdir: wd}, domain.ExposeEntry{Path: domain.ExposeAuto}, tc.scan, tc.suffix)

			require.NoError(t, err)
			assert.Empty(t, got)
			assert.Equal(t, models.ReasonNoDesktop, reason)
		})
	}
}

func TestLaunchers_ScanError(t *testing.T) {
	mocks.RequireUnixHost(t)
	if os.Geteuid() == 0 {
		t.Skip("root reads unreadable directories")
	}
	sb := mocks.NewSandbox(t, "linux")
	wd := sb.Workdir(t, mocks.NsA)
	locked := filepath.Join(wd, "locked")
	require.NoError(t, os.MkdirAll(locked, 0o750))
	require.NoError(t, os.Chmod(locked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o750) })

	_, _, err := Launchers(models.Request{Bare: mocks.BareA, Workdir: wd}, domain.ExposeEntry{Name: "tool", Path: domain.ExposeAuto}, unixScan(), models.AppImageExt)

	require.Error(t, err)
}

func TestLaunchers_MissingWorkdir(t *testing.T) {
	req := models.Request{Bare: mocks.BareA, Workdir: filepath.Join(t.TempDir(), "missing")}

	_, _, err := Launchers(req, domain.ExposeEntry{Name: "x", Path: domain.ExposeAuto}, unixScan(), models.AppImageExt)

	require.Error(t, err)
}

func TestNative_Directories(t *testing.T) {
	wd := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(wd, "Tool.app"), 0o750))
	mocks.WriteFile(t, filepath.Join(wd, "File.app"), "x", 0o644)

	got, err := Native(wd, models.BundleExt, true)

	require.NoError(t, err)
	assert.Equal(t, []models.Candidate{{Name: "Tool", Target: filepath.Join(wd, "Tool.app"), Depth: 1}}, got)
}

func TestNested_Bundles(t *testing.T) {
	wd := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(wd, "wrap", "Tool.app"), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(wd, "Top.app", "Inner.app"), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(wd, ".hidden", "Ghost.app"), 0o750))

	got, err := Nested(wd, models.BundleExt)

	require.NoError(t, err)
	assert.Equal(t, []models.Candidate{{Name: "Tool", Target: filepath.Join(wd, "wrap", "Tool.app"), Depth: 2}}, got)
}

func TestNested_MissingWorkdir(t *testing.T) {
	_, err := Nested(filepath.Join(t.TempDir(), "missing"), models.BundleExt)

	require.Error(t, err)
}

func TestNested_UnreadableWrapper(t *testing.T) {
	mocks.RequireUnixHost(t)
	if os.Geteuid() == 0 {
		t.Skip("root reads unreadable directories")
	}
	wd := t.TempDir()
	locked := filepath.Join(wd, "locked")
	require.NoError(t, os.MkdirAll(locked, 0o750))
	require.NoError(t, os.Chmod(locked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o750) })

	_, err := Nested(wd, models.BundleExt)

	require.Error(t, err)
}

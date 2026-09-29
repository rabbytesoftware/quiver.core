package resolve

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/ownership"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/platform"
)

type fixture struct {
	*mocks.Sandbox
	resolver *resolver
}

func newFixture(
	t *testing.T,
	goos string,
) *fixture {
	t.Helper()
	sb := mocks.NewSandbox(t, goos)
	return &fixture{
		Sandbox:  sb,
		resolver: New(sb.Host(), ownership.NewBundles(sb.Tagger)).(*resolver),
	}
}

func TestResolver_Resolve_Declared(t *testing.T) {
	f := newFixture(t, "linux")
	wd := f.Workdir(t, mocks.NsA)
	req := models.ApplyRequest{Bare: mocks.BareA, Workdir: wd}

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
			got, reason, err := f.resolver.Resolve(req, domain.ExposeKindCLI, tc.entry)
			require.NoError(t, err)
			assert.Equal(t, tc.wantReason, reason)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestResolver_Resolve_AutoCLI(t *testing.T) {
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
			name: "skips bundles hidden dirs and deep files",
			files: map[string]os.FileMode{
				"App.app/Contents/MacOS/app": 0o755,
				".git/hooks/pre":             0o755,
				"a/b/c/d/deep":               0o755,
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
			wd := f.Workdir(t, mocks.NsA)
			for rel, mode := range tc.files {
				mocks.WriteFile(t, filepath.Join(wd, filepath.FromSlash(rel)), "x", mode)
			}
			req := models.ApplyRequest{Bare: mocks.BareA, Workdir: wd}

			got, reason, err := f.resolver.Resolve(req, domain.ExposeKindCLI, domain.ExposeEntry{Name: tc.entryName, Path: domain.ExposeAuto})

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

func TestResolver_Resolve_AutoCLI_LinuxAppSuffixedBundle(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newFixture(t, "linux")
	wd := f.Workdir(t, mocks.NsA)
	mocks.WriteFile(t, filepath.Join(wd, "zed.app", "bin", "zed"), "x", 0o755)
	mocks.WriteFile(t, filepath.Join(wd, "zed.app", "libexec", "zed-editor"), "x", 0o755)
	req := models.ApplyRequest{Bare: "github.com/zed-industries/zed", Workdir: wd}

	got, reason, err := f.resolver.Resolve(req, domain.ExposeKindCLI, domain.ExposeEntry{Name: "zed", Path: domain.ExposeAuto})

	require.NoError(t, err)
	assert.Empty(t, reason)
	require.Len(t, got, 1)
	assert.Equal(t, filepath.Join(wd, "zed.app", "bin", "zed"), got[0].Target)
	assert.Equal(t, "zed", got[0].Name)
	assert.Equal(t, 3, got[0].Depth)
}

func TestResolver_Resolve_AutoCLI_DarwinSkipsAppBundle(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newFixture(t, platform.GOOSDarwin)
	wd := f.Workdir(t, mocks.NsA)
	mocks.WriteFile(t, filepath.Join(wd, "Foo.app", "foo"), "x", 0o755)
	req := models.ApplyRequest{Bare: mocks.BareA, Workdir: wd}

	got, reason, err := f.resolver.Resolve(req, domain.ExposeKindCLI, domain.ExposeEntry{Path: domain.ExposeAuto})

	require.NoError(t, err)
	assert.Equal(t, models.ReasonNoExecutable, reason)
	assert.Empty(t, got)
}

func TestResolver_Resolve_AutoCLI_Windows(t *testing.T) {
	f := newFixture(t, platform.GOOSWindows)
	wd := f.Workdir(t, mocks.NsA)
	mocks.WriteFile(t, filepath.Join(wd, "bin", "rg.EXE"), "x", 0o644)
	mocks.WriteFile(t, filepath.Join(wd, "bin", "notes.txt"), "x", 0o755)
	req := models.ApplyRequest{Bare: mocks.BareA, Workdir: wd}

	got, reason, err := f.resolver.Resolve(req, domain.ExposeKindCLI, domain.ExposeEntry{Path: domain.ExposeAuto})

	require.NoError(t, err)
	assert.Empty(t, reason)
	assert.Equal(t, []models.Candidate{{Name: "rg", Target: filepath.Join(wd, "bin", "rg.EXE"), Depth: 2}}, got)
}

func TestResolver_Resolve_AutoDesktop(t *testing.T) {
	testCases := []struct {
		name       string
		goos       string
		dirs       []string
		files      []string
		placed     map[string]domain.Namespace
		entryName  string
		want       *models.Candidate
		wantReason string
	}{
		{
			name:      "darwin single bundle keeps its own name",
			goos:      platform.GOOSDarwin,
			dirs:      []string{"CC Switch.app"},
			entryName: "cc-switch",
			want:      &models.Candidate{Name: "CC Switch", Target: "CC Switch.app", Depth: 1},
		},
		{
			name:      "darwin bundle already moved resolves to the bundle this arrow placed",
			goos:      platform.GOOSDarwin,
			placed:    map[string]domain.Namespace{"CC Switch.app": mocks.BareA, "Other.app": mocks.BareB},
			entryName: "cc-switch",
			want:      &models.Candidate{Name: "CC Switch", Target: "CC Switch.app", Depth: 1},
		},
		{
			name:       "darwin bundle placed by another arrow is not ours",
			goos:       platform.GOOSDarwin,
			placed:     map[string]domain.Namespace{"Other.app": mocks.BareB},
			entryName:  "Tool",
			wantReason: models.ReasonNoDesktop,
		},
		{
			name:       "darwin nothing and no name",
			goos:       platform.GOOSDarwin,
			wantReason: models.ReasonNoDesktop,
		},
		{
			name: "darwin several bundles pick the repo name",
			goos: platform.GOOSDarwin,
			dirs: []string{"Other.app", "Tool.app"},
			want: &models.Candidate{Name: "Tool", Target: "Tool.app", Depth: 1},
		},
		{
			name:       "darwin several bundles without a match",
			goos:       platform.GOOSDarwin,
			dirs:       []string{"One.app", "Two.app"},
			wantReason: models.ReasonAmbiguous,
		},
		{
			name:      "linux appimage",
			goos:      "linux",
			dirs:      []string{"dir.AppImage"},
			files:     []string{"Tool-x86_64.AppImage", "notes.txt"},
			entryName: "Tool",
			want:      &models.Candidate{Name: "Tool", Target: "Tool-x86_64.AppImage", Depth: 1},
		},
		{
			name:      "linux several appimages pick the entry name",
			goos:      "linux",
			files:     []string{"A.AppImage", "B.AppImage"},
			entryName: "B",
			want:      &models.Candidate{Name: "B", Target: "B.AppImage", Depth: 1},
		},
		{
			name:       "linux ignores bundles",
			goos:       "linux",
			dirs:       []string{"Tool.app"},
			entryName:  "Tool",
			wantReason: models.ReasonNoDesktop,
		},
		{
			name:  "windows single exe",
			goos:  platform.GOOSWindows,
			files: []string{"setup.exe"},
			want:  &models.Candidate{Name: "setup", Target: "setup.exe", Depth: 1},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, tc.goos)
			wd := f.Workdir(t, mocks.NsA)
			for _, d := range tc.dirs {
				require.NoError(t, os.MkdirAll(filepath.Join(wd, d), 0o750))
			}
			for _, file := range tc.files {
				mocks.WriteFile(t, filepath.Join(wd, file), "x", 0o755)
			}
			for bundle, owner := range tc.placed {
				path := filepath.Join(f.Apps[0], bundle)
				require.NoError(t, os.MkdirAll(path, 0o750))
				require.NoError(t, f.Tagger.Write(path, ownership.BundleTag(owner, "/wd", path)))
			}
			mocks.WriteFile(t, filepath.Join(f.Apps[0], "Stray.app"), "x", 0o644)
			req := models.ApplyRequest{Bare: mocks.BareA, Workdir: wd, Layout: platform.Layout{Apps: f.Apps}}

			got, reason, err := f.resolver.Resolve(req, domain.ExposeKindDesktop, domain.ExposeEntry{Name: tc.entryName, Path: domain.ExposeAuto})

			require.NoError(t, err)
			assert.Equal(t, tc.wantReason, reason)
			if tc.want == nil {
				assert.Empty(t, got)
				return
			}
			want := *tc.want
			want.Target = filepath.Join(wd, want.Target)
			assert.Equal(t, []models.Candidate{want}, got)
		})
	}
}

func TestResolver_Resolve_AutoDesktop_IgnoresSymlinks(t *testing.T) {
	mocks.RequireUnixHost(t)

	testCases := []struct {
		name string
		goos string
		link string
		dir  bool
	}{
		{name: "linux appimage", goos: "linux", link: "X.AppImage"},
		{name: "windows exe", goos: platform.GOOSWindows, link: "x.exe"},
		{name: "darwin bundle", goos: platform.GOOSDarwin, link: "X.app", dir: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, tc.goos)
			wd := f.Workdir(t, mocks.NsA)
			outside := filepath.Join(t.TempDir(), tc.link)
			if tc.dir {
				require.NoError(t, os.MkdirAll(outside, 0o750))
			}
			if !tc.dir {
				mocks.WriteFile(t, outside, "x", 0o755)
			}
			require.NoError(t, os.Symlink(outside, filepath.Join(wd, tc.link)))

			got, reason, err := f.resolver.Resolve(models.ApplyRequest{Bare: mocks.BareA, Workdir: wd}, domain.ExposeKindDesktop, domain.ExposeEntry{Path: domain.ExposeAuto})

			require.NoError(t, err)
			assert.Empty(t, got)
			assert.Equal(t, models.ReasonNoDesktop, reason)
		})
	}
}

func TestResolver_Resolve_AutoDesktop_ScanFallback(t *testing.T) {
	testCases := []struct {
		name       string
		goos       string
		bare       domain.Namespace
		files      map[string]os.FileMode
		entryName  string
		want       *models.Candidate
		wantReason string
	}{
		{
			name:      "linux archive tree resolves the executable named after the repo",
			goos:      "linux",
			bare:      "github.com/logseq/logseq",
			files:     map[string]os.FileMode{"logseq/Logseq": 0o755, "logseq/chrome-sandbox": 0o755},
			entryName: "logseq-desktop",
			want:      &models.Candidate{Name: "logseq-desktop", Target: "logseq/Logseq", Depth: 2},
		},
		{
			name:      "linux archive tree resolves the executable named after the entry",
			goos:      "linux",
			bare:      "github.com/acme/suite",
			files:     map[string]os.FileMode{"suite-1.0/editor": 0o755, "suite-1.0/viewer": 0o755},
			entryName: "viewer",
			want:      &models.Candidate{Name: "viewer", Target: "suite-1.0/viewer", Depth: 2},
		},
		{
			name:      "windows archive tree resolves the exe named after the repo",
			goos:      platform.GOOSWindows,
			bare:      "github.com/obsproject/obs",
			files:     map[string]os.FileMode{"app/OBS.exe": 0o644, "app/helper.exe": 0o644},
			entryName: "obs-studio",
			want:      &models.Candidate{Name: "obs-studio", Target: "app/OBS.exe", Depth: 2},
		},
		{
			name:       "linux executables without a matching name are not desktop apps",
			goos:       "linux",
			bare:       "github.com/logseq/logseq",
			files:      map[string]os.FileMode{"bin/helper": 0o755},
			entryName:  "logseq-desktop",
			wantReason: models.ReasonNoDesktop,
		},
		{
			name:      "linux app suffixed archive tree is scanned like any directory",
			goos:      "linux",
			bare:      "github.com/zed-industries/zed",
			files:     map[string]os.FileMode{"zed.app/bin/zed": 0o755, "zed.app/libexec/zed-editor": 0o755},
			entryName: "zed",
			want:      &models.Candidate{Name: "zed", Target: "zed.app/bin/zed", Depth: 3},
		},
		{
			name:       "darwin gets no scan fallback",
			goos:       platform.GOOSDarwin,
			bare:       "github.com/logseq/logseq",
			files:      map[string]os.FileMode{"logseq/Logseq": 0o755},
			entryName:  "logseq",
			wantReason: models.ReasonNoDesktop,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.goos != platform.GOOSWindows {
				mocks.RequireUnixHost(t)
			}
			f := newFixture(t, tc.goos)
			wd := f.Workdir(t, mocks.NsA)
			for rel, mode := range tc.files {
				mocks.WriteFile(t, filepath.Join(wd, filepath.FromSlash(rel)), "x", mode)
			}
			req := models.ApplyRequest{Bare: tc.bare, Workdir: wd, Layout: platform.Layout{Apps: f.Apps}}

			got, reason, err := f.resolver.Resolve(req, domain.ExposeKindDesktop, domain.ExposeEntry{Name: tc.entryName, Path: domain.ExposeAuto})

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

func TestResolver_Resolve_AutoDesktop_ScanError(t *testing.T) {
	mocks.RequireUnixHost(t)
	if os.Geteuid() == 0 {
		t.Skip("root reads unreadable directories")
	}
	f := newFixture(t, "linux")
	wd := f.Workdir(t, mocks.NsA)
	locked := filepath.Join(wd, "locked")
	require.NoError(t, os.MkdirAll(locked, 0o750))
	require.NoError(t, os.Chmod(locked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o750) })

	_, _, err := f.resolver.Resolve(models.ApplyRequest{Bare: mocks.BareA, Workdir: wd}, domain.ExposeKindDesktop, domain.ExposeEntry{Name: "tool", Path: domain.ExposeAuto})

	require.Error(t, err)
}

func TestResolver_Resolve_Declared_ContainmentError(t *testing.T) {
	f := newFixture(t, "linux")
	req := models.ApplyRequest{Bare: mocks.BareA, Workdir: filepath.Join(t.TempDir(), "missing")}

	_, _, err := f.resolver.Resolve(req, domain.ExposeKindCLI, domain.ExposeEntry{Name: "rg", Path: "rg"})

	require.Error(t, err)
}

func TestResolver_Resolve_AutoDesktop_MissingWorkdir(t *testing.T) {
	f := newFixture(t, "linux")
	req := models.ApplyRequest{Bare: mocks.BareA, Workdir: filepath.Join(t.TempDir(), "missing")}

	_, _, err := f.resolver.Resolve(req, domain.ExposeKindDesktop, domain.ExposeEntry{Name: "x", Path: domain.ExposeAuto})

	require.Error(t, err)
}

func TestRepoName(t *testing.T) {
	assert.Equal(t, "tool", repoName(mocks.BareA))
	assert.Equal(t, "r", repoName("q.io/u/r/auid"))
	assert.Empty(t, repoName("short/ns"))
}

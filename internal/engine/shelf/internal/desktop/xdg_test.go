package desktop

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/fsguard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/platform"
)

func TestXDGFileName(t *testing.T) {
	assert.Equal(t, "quiver-841b8a1d33c8-My-App.desktop", xdgFileName("github.com/u/r", "My App"))
	assert.NotEqual(t, xdgFileName("github.com/a-b/c", "x"), xdgFileName("github.com/a/b-c", "x"))
}

func TestXDGContent(t *testing.T) {
	testCases := []struct {
		name       string
		target     string
		icon       string
		categories []string
		want       string
	}{
		{
			name:       "full",
			target:     "/wd/Tool.AppImage",
			icon:       "/wd/icon.png",
			categories: []string{"Development", "IDE"},
			want: "[Desktop Entry]\nType=Application\nName=Tool\nExec=\"/wd/Tool.AppImage\" %U\nIcon=/wd/icon.png\n" +
				"Terminal=false\nCategories=Development;IDE;\nX-Quiver-Namespace=github.com/u/r\n",
		},
		{
			name:   "minimal with escaped percent",
			target: "/wd/100%/Tool",
			want: "[Desktop Entry]\nType=Application\nName=Tool\nExec=\"/wd/100%%/Tool\" %U\n" +
				"Terminal=false\nX-Quiver-Namespace=github.com/u/r\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, xdgContent("github.com/u/r", "Tool", tc.target, tc.icon, tc.categories))
		})
	}
}

func TestDesktopIcon(t *testing.T) {
	wd := filepath.Join(t.TempDir(), "wd")

	testCases := []struct {
		name      string
		entry     string
		candidate string
		media     string
		want      string
	}{
		{name: "entry icon", entry: "${INSTALL_PATH}/icon.png", candidate: "/c.png", media: "/m.png", want: filepath.Join(wd, "icon.png")},
		{name: "candidate icon over media", candidate: "/c.png", media: "/m.png", want: "/c.png"},
		{name: "media path", media: "/m.png", want: "/m.png"},
		{name: "media url", media: "https://raw.example/icon.png", want: ""},
		{name: "control characters", media: "/m\n.png", want: ""},
		{name: "none", want: ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := models.ApplyRequest{Workdir: wd, Media: domain.ArrowMedia{Icon: tc.media}}
			assert.Equal(t, tc.want, desktopIcon(req, domain.ExposeEntry{Icon: tc.entry}, models.Candidate{Icon: tc.candidate}))
		})
	}
}

func TestPlaceXDG_RecordCandidate(t *testing.T) {
	f := newFixture(t, "linux")
	wd := f.Workdir(t, mocks.NsA)
	target := filepath.Join(wd, "Logseq.AppDir", ".quiver-run")
	mocks.WriteFile(t, target, "x", 0o755)
	icon := filepath.Join(wd, "Logseq.AppDir", "logseq.png")
	l := f.Layout(t)
	req := models.ApplyRequest{Layout: l, Bare: mocks.BareA, Workdir: wd, Media: domain.ArrowMedia{Icon: "/media.png"}}
	c := models.Candidate{Name: "logseq", Display: "Logseq", Target: target, Icon: icon}

	got, err := placeXDG(req, domain.ExposeEntry{Name: "logseq"}, c)

	require.NoError(t, err)
	loc := filepath.Join(xdgDir(f.UserHome), xdgFileName(mocks.BareA, "logseq"))
	assert.Equal(t, models.Placement{Location: loc}, got)
	data, err := os.ReadFile(loc)
	require.NoError(t, err)
	assert.Equal(t, xdgContent(mocks.BareA, "Logseq", target, icon, []string{}), string(data))
}

func TestDesktopCategories(t *testing.T) {
	assert.Equal(t, []string{"C", "D"}, desktopCategories([]string{"A;B", "", "C", "x\ny", "D", "E=F"}))
}

func TestPlaceXDG(t *testing.T) {
	f := newFixture(t, "linux")
	wd := f.Workdir(t, mocks.NsA)
	target := filepath.Join(wd, "Tool.AppImage")
	mocks.WriteFile(t, target, "x", 0o644)
	dollar := filepath.Join(wd, "$Tool")
	mocks.WriteFile(t, dollar, "x", 0o755)
	l := f.Layout(t)
	req := models.ApplyRequest{Layout: l, Bare: mocks.BareA, Workdir: wd}
	entry := domain.ExposeEntry{Name: "Tool", Icon: "${INSTALL_PATH}/icon.png", Categories: []string{"Development"}}
	loc := filepath.Join(xdgDir(f.UserHome), xdgFileName(mocks.BareA, "Tool"))

	testCases := []struct {
		name     string
		existing string
		target   string
		want     models.Placement
	}{
		{name: "new entry", target: target, want: models.Placement{Location: loc}},
		{name: "own entry", existing: "X-Quiver-Namespace=github.com/acme/tool\n", target: target, want: models.Placement{Location: loc}},
		{name: "foreign entry", existing: "X-Quiver-Namespace=github.com/other/thing\n", target: target, want: models.Placement{Refused: "owned by github.com/other/thing"}},
		{name: "user entry", existing: "[Desktop Entry]\n", target: target, want: models.Placement{Refused: models.ReasonUnmanaged}},
		{name: "unsafe target", target: dollar, want: models.Placement{Refused: models.ReasonUnsafePath}},
		{name: "missing target", target: filepath.Join(wd, "missing"), want: models.Placement{Refused: models.ReasonNotFound}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, os.RemoveAll(loc))
			if tc.existing != "" {
				mocks.WriteFile(t, loc, tc.existing, 0o600)
			}

			got, err := placeXDG(req, entry, models.Candidate{Name: "Tool", Target: tc.target})

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
			if tc.want.Location == "" {
				return
			}
			data, err := os.ReadFile(loc)
			require.NoError(t, err)
			assert.Equal(t, xdgContent(mocks.BareA, "Tool", target, filepath.Join(wd, "icon.png"), []string{"Development"}), string(data))
		})
	}
}

func TestPlaceXDG_EntryAndDirAreWorldReadable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes are not represented on windows")
	}
	f := newFixture(t, "linux")
	wd := f.Workdir(t, mocks.NsA)
	target := filepath.Join(wd, "Tool.AppImage")
	mocks.WriteFile(t, target, "x", 0o644)
	req := models.ApplyRequest{Layout: f.Layout(t), Bare: mocks.BareA, Workdir: wd}

	got, err := placeXDG(req, domain.ExposeEntry{Name: "Tool"}, models.Candidate{Name: "Tool", Target: target})

	require.NoError(t, err)
	fileInfo, err := os.Stat(got.Location)
	require.NoError(t, err)
	dirInfo, err := os.Stat(xdgDir(f.UserHome))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o044), fileInfo.Mode().Perm()&0o044)
	assert.Equal(t, os.FileMode(0o055), dirInfo.Mode().Perm()&0o055)
}

func TestPlaceXDG_MarksAppImageExecutable(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newFixture(t, "linux")
	wd := f.Workdir(t, mocks.NsA)
	target := filepath.Join(wd, "Tool.AppImage")
	mocks.WriteFile(t, target, "x", 0o644)
	plain := filepath.Join(wd, "tool")
	mocks.WriteFile(t, plain, "x", 0o644)
	l := f.Layout(t)
	req := models.ApplyRequest{Layout: l, Bare: mocks.BareA, Workdir: wd}

	_, err := placeXDG(req, domain.ExposeEntry{}, models.Candidate{Name: "Tool", Target: target})
	require.NoError(t, err)
	_, err = placeXDG(req, domain.ExposeEntry{}, models.Candidate{Name: "tool", Target: plain})
	require.NoError(t, err)

	info, err := os.Stat(target)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
	info, err = os.Stat(plain)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())
}

func TestPlaceXDG_Errors(t *testing.T) {
	f := newFixture(t, "linux")
	wd := f.Workdir(t, mocks.NsA)
	target := filepath.Join(wd, "tool")
	mocks.WriteFile(t, target, "x", 0o755)
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	_, err := placeXDG(models.ApplyRequest{Layout: platform.Layout{UserHome: file}, Bare: mocks.BareA, Workdir: wd}, domain.ExposeEntry{}, models.Candidate{Name: "tool", Target: target})
	require.Error(t, err)

	mocks.RequireUnixHost(t)
	if os.Geteuid() == 0 {
		return
	}
	home := t.TempDir()
	mocks.WriteFile(t, filepath.Join(xdgDir(home), xdgFileName(mocks.BareA, "tool")), "x", 0o000)

	_, err = placeXDG(models.ApplyRequest{Layout: platform.Layout{UserHome: home}, Bare: mocks.BareA, Workdir: wd}, domain.ExposeEntry{}, models.Candidate{Name: "tool", Target: target})
	require.Error(t, err)
}

func TestPlaceXDG_SwapError(t *testing.T) {
	f := newFixture(t, "linux")
	wd := f.Workdir(t, mocks.NsA)
	target := filepath.Join(wd, "tool")
	mocks.WriteFile(t, target, "x", 0o755)
	l := f.Layout(t)
	loc := filepath.Join(xdgDir(f.UserHome), xdgFileName(mocks.BareA, "tool"))
	mocks.WriteFile(t, filepath.Join(loc+fsguard.StagedSuffix, "blocker"), "x", 0o600)

	_, err := placeXDG(models.ApplyRequest{Layout: l, Bare: mocks.BareA, Workdir: wd}, domain.ExposeEntry{}, models.Candidate{Name: "tool", Target: target})

	require.Error(t, err)
}

func TestRemoveXDG(t *testing.T) {
	f := newFixture(t, "linux")
	dir := xdgDir(f.UserHome)
	own := filepath.Join(dir, xdgFileName(mocks.BareA, "Tool"))
	foreign := filepath.Join(dir, xdgFileName(mocks.BareB, "Tool"))
	user := filepath.Join(dir, "user.desktop")
	mocks.WriteFile(t, own, xdgContent(mocks.BareA, "Tool", "/x", "", nil), 0o600)
	mocks.WriteFile(t, foreign, xdgContent(mocks.BareB, "Tool", "/x", "", nil), 0o600)
	mocks.WriteFile(t, user, xdgContent(mocks.BareA, "Tool", "/x", "", nil), 0o600)
	kept := filepath.Join(dir, xdgFileName(mocks.BareA, "Kept"))
	mocks.WriteFile(t, kept, xdgContent(mocks.BareA, "Kept", "/x", "", nil), 0o600)

	require.NoError(t, removeXDG(platform.Layout{UserHome: f.UserHome}, mocks.BareA, map[string]bool{kept: true}))

	assert.NoFileExists(t, own)
	assert.FileExists(t, foreign)
	assert.FileExists(t, user)
	assert.FileExists(t, kept)
}

func TestMarkAppImage_SkipsTargetsResolvingOutside(t *testing.T) {
	mocks.RequireUnixHost(t)
	wd := t.TempDir()
	outside := filepath.Join(t.TempDir(), "X.AppImage")
	mocks.WriteFile(t, outside, "x", 0o644)
	link := filepath.Join(wd, "X.AppImage")
	require.NoError(t, os.Symlink(outside, link))

	require.NoError(t, markAppImage(wd, link))

	info, err := os.Stat(outside)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())
}

func TestRemoveXDG_Errors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	blocked := t.TempDir()
	mocks.WriteFile(t, filepath.Join(blocked, ".local", "share", "applications"), "", 0o600)

	assert.NoError(t, removeXDG(platform.Layout{UserHome: missing}, mocks.BareA, nil))
	assert.Error(t, removeXDG(platform.Layout{UserHome: blocked}, mocks.BareA, nil))

	mocks.RequireUnixHost(t)
	if os.Geteuid() == 0 {
		return
	}
	home := t.TempDir()
	unreadable := filepath.Join(xdgDir(home), xdgFileName(mocks.BareA, "x"))
	mocks.WriteFile(t, unreadable, xdgContent(mocks.BareA, "x", "/x", "", nil), 0o000)

	assert.Error(t, removeXDG(platform.Layout{UserHome: home}, mocks.BareA, nil))

	require.NoError(t, os.Chmod(unreadable, 0o600))
	require.NoError(t, os.Chmod(xdgDir(home), 0o500))
	t.Cleanup(func() { _ = os.Chmod(xdgDir(home), 0o750) })

	assert.Error(t, removeXDG(platform.Layout{UserHome: home}, mocks.BareA, nil))
}

func TestPlaceXDG_CreateDirError(t *testing.T) {
	f := newFixture(t, "linux")
	wd := f.Workdir(t, mocks.NsA)
	target := filepath.Join(wd, "tool")
	mocks.WriteFile(t, target, "x", 0o755)
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	req := models.ApplyRequest{Layout: platform.Layout{UserHome: file}, Bare: mocks.BareA, Workdir: wd}

	_, err := placeXDG(req, domain.ExposeEntry{}, models.Candidate{Name: "tool", Target: target})

	require.Error(t, err)
}

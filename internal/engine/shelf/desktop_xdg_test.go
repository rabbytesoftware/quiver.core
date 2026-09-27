package shelf

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
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
			req := applyRequest{workdir: wd, media: domain.ArrowMedia{Icon: tc.media}}
			assert.Equal(t, tc.want, desktopIcon(req, domain.ExposeEntry{Icon: tc.entry}, candidate{icon: tc.candidate}))
		})
	}
}

func TestPlaceXDG_RecordCandidate(t *testing.T) {
	f := newFixture(t, "linux")
	wd := f.workdir(t, nsA)
	target := filepath.Join(wd, "Logseq.AppDir", ".quiver-run")
	writeFile(t, target, "x", 0o755)
	icon := filepath.Join(wd, "Logseq.AppDir", "logseq.png")
	l, err := f.shelf.layout()
	require.NoError(t, err)
	req := applyRequest{layout: l, bare: bareA, workdir: wd, media: domain.ArrowMedia{Icon: "/media.png"}}
	c := candidate{name: "logseq", display: "Logseq", target: target, icon: icon}

	got, err := placeXDG(req, domain.ExposeEntry{Name: "logseq"}, c)

	require.NoError(t, err)
	loc := filepath.Join(xdgDir(f.userHome), xdgFileName(bareA, "logseq"))
	assert.Equal(t, placement{location: loc}, got)
	data, err := os.ReadFile(loc)
	require.NoError(t, err)
	assert.Equal(t, xdgContent(bareA, "Logseq", target, icon, []string{}), string(data))
}

func TestCandidate_DisplayName(t *testing.T) {
	assert.Equal(t, "Logseq", candidate{name: "logseq", display: "Logseq"}.displayName())
	assert.Equal(t, "logseq", candidate{name: "logseq"}.displayName())
}

func TestDesktopCategories(t *testing.T) {
	assert.Equal(t, []string{"C", "D"}, desktopCategories([]string{"A;B", "", "C", "x\ny", "D", "E=F"}))
}

func TestPlaceXDG(t *testing.T) {
	f := newFixture(t, "linux")
	wd := f.workdir(t, nsA)
	target := filepath.Join(wd, "Tool.AppImage")
	writeFile(t, target, "x", 0o644)
	dollar := filepath.Join(wd, "$Tool")
	writeFile(t, dollar, "x", 0o755)
	l, err := f.shelf.layout()
	require.NoError(t, err)
	req := applyRequest{layout: l, bare: bareA, workdir: wd}
	entry := domain.ExposeEntry{Name: "Tool", Icon: "${INSTALL_PATH}/icon.png", Categories: []string{"Development"}}
	loc := filepath.Join(xdgDir(f.userHome), xdgFileName(bareA, "Tool"))

	testCases := []struct {
		name     string
		existing string
		target   string
		want     placement
	}{
		{name: "new entry", target: target, want: placement{location: loc}},
		{name: "own entry", existing: "X-Quiver-Namespace=github.com/acme/tool\n", target: target, want: placement{location: loc}},
		{name: "foreign entry", existing: "X-Quiver-Namespace=github.com/other/thing\n", target: target, want: placement{refused: "owned by github.com/other/thing"}},
		{name: "user entry", existing: "[Desktop Entry]\n", target: target, want: placement{refused: reasonUnmanaged}},
		{name: "unsafe target", target: dollar, want: placement{refused: reasonUnsafePath}},
		{name: "missing target", target: filepath.Join(wd, "missing"), want: placement{refused: reasonNotFound}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, os.RemoveAll(loc))
			if tc.existing != "" {
				writeFile(t, loc, tc.existing, 0o600)
			}

			got, err := placeXDG(req, entry, candidate{name: "Tool", target: tc.target})

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
			if tc.want.location == "" {
				return
			}
			data, err := os.ReadFile(loc)
			require.NoError(t, err)
			assert.Equal(t, xdgContent(bareA, "Tool", target, filepath.Join(wd, "icon.png"), []string{"Development"}), string(data))
		})
	}
}

func TestPlaceXDG_MarksAppImageExecutable(t *testing.T) {
	requireUnixHost(t)
	f := newFixture(t, "linux")
	wd := f.workdir(t, nsA)
	target := filepath.Join(wd, "Tool.AppImage")
	writeFile(t, target, "x", 0o644)
	plain := filepath.Join(wd, "tool")
	writeFile(t, plain, "x", 0o644)
	l, err := f.shelf.layout()
	require.NoError(t, err)
	req := applyRequest{layout: l, bare: bareA, workdir: wd}

	_, err = placeXDG(req, domain.ExposeEntry{}, candidate{name: "Tool", target: target})
	require.NoError(t, err)
	_, err = placeXDG(req, domain.ExposeEntry{}, candidate{name: "tool", target: plain})
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
	wd := f.workdir(t, nsA)
	target := filepath.Join(wd, "tool")
	writeFile(t, target, "x", 0o755)
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	_, err := placeXDG(applyRequest{layout: layout{userHome: file}, bare: bareA, workdir: wd}, domain.ExposeEntry{}, candidate{name: "tool", target: target})
	require.Error(t, err)

	requireUnixHost(t)
	if os.Geteuid() == 0 {
		return
	}
	home := t.TempDir()
	writeFile(t, filepath.Join(xdgDir(home), xdgFileName(bareA, "tool")), "x", 0o000)

	_, err = placeXDG(applyRequest{layout: layout{userHome: home}, bare: bareA, workdir: wd}, domain.ExposeEntry{}, candidate{name: "tool", target: target})
	require.Error(t, err)
}

func TestPlaceXDG_SwapError(t *testing.T) {
	f := newFixture(t, "linux")
	wd := f.workdir(t, nsA)
	target := filepath.Join(wd, "tool")
	writeFile(t, target, "x", 0o755)
	l, err := f.shelf.layout()
	require.NoError(t, err)
	loc := filepath.Join(xdgDir(f.userHome), xdgFileName(bareA, "tool"))
	writeFile(t, filepath.Join(loc+stagedSuffix, "blocker"), "x", 0o600)

	_, err = placeXDG(applyRequest{layout: l, bare: bareA, workdir: wd}, domain.ExposeEntry{}, candidate{name: "tool", target: target})

	require.Error(t, err)
}

func TestRemoveXDG(t *testing.T) {
	f := newFixture(t, "linux")
	dir := xdgDir(f.userHome)
	own := filepath.Join(dir, xdgFileName(bareA, "Tool"))
	foreign := filepath.Join(dir, xdgFileName(bareB, "Tool"))
	user := filepath.Join(dir, "user.desktop")
	writeFile(t, own, xdgContent(bareA, "Tool", "/x", "", nil), 0o600)
	writeFile(t, foreign, xdgContent(bareB, "Tool", "/x", "", nil), 0o600)
	writeFile(t, user, xdgContent(bareA, "Tool", "/x", "", nil), 0o600)
	kept := filepath.Join(dir, xdgFileName(bareA, "Kept"))
	writeFile(t, kept, xdgContent(bareA, "Kept", "/x", "", nil), 0o600)

	require.NoError(t, removeXDG(layout{userHome: f.userHome}, bareA, map[string]bool{kept: true}))

	assert.NoFileExists(t, own)
	assert.FileExists(t, foreign)
	assert.FileExists(t, user)
	assert.FileExists(t, kept)
}

func TestMarkAppImage_SkipsTargetsResolvingOutside(t *testing.T) {
	requireUnixHost(t)
	wd := t.TempDir()
	outside := filepath.Join(t.TempDir(), "X.AppImage")
	writeFile(t, outside, "x", 0o644)
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
	writeFile(t, filepath.Join(blocked, ".local", "share", "applications"), "", 0o600)

	assert.NoError(t, removeXDG(layout{userHome: missing}, bareA, nil))
	assert.Error(t, removeXDG(layout{userHome: blocked}, bareA, nil))

	requireUnixHost(t)
	if os.Geteuid() == 0 {
		return
	}
	home := t.TempDir()
	unreadable := filepath.Join(xdgDir(home), xdgFileName(bareA, "x"))
	writeFile(t, unreadable, xdgContent(bareA, "x", "/x", "", nil), 0o000)

	assert.Error(t, removeXDG(layout{userHome: home}, bareA, nil))

	require.NoError(t, os.Chmod(unreadable, 0o600))
	require.NoError(t, os.Chmod(xdgDir(home), 0o500))
	t.Cleanup(func() { _ = os.Chmod(xdgDir(home), 0o750) })

	assert.Error(t, removeXDG(layout{userHome: home}, bareA, nil))
}

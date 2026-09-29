package xdg

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/discover"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/fsguard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/mocks"
)

type fixture struct {
	*mocks.Sandbox
	exposer *exposer
}

func newFixture(
	t *testing.T,
) *fixture {
	t.Helper()
	sb := mocks.NewSandbox(t, "linux")
	return &fixture{Sandbox: sb, exposer: New(sb.Host(), discover.Scan{Rule: discover.ExecBits()}).(*exposer)}
}

func removeEntries(
	l models.Layout,
	claim models.Claim,
	keep map[string]bool,
) error {
	return (&exposer{}).Remove(context.Background(), l, claim, keep)
}

func TestFileName(t *testing.T) {
	assert.Equal(t, "quiver-841b8a1d33c8-My-App.desktop", fileName("github.com/u/r", "My App"))
	assert.NotEqual(t, fileName("github.com/a-b/c", "x"), fileName("github.com/a/b-c", "x"))
}

func TestContent(t *testing.T) {
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
				"Terminal=false\nCategories=Development;IDE;\nX-Quiver-Namespace=github.com/u/r\nX-Quiver-Workdir=/wd\n",
		},
		{
			name:   "minimal with escaped percent",
			target: "/wd/100%/Tool",
			want: "[Desktop Entry]\nType=Application\nName=Tool\nExec=\"/wd/100%%/Tool\" %U\n" +
				"Terminal=false\nX-Quiver-Namespace=github.com/u/r\nX-Quiver-Workdir=/wd\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, content("github.com/u/r", "/wd", "Tool", tc.target, tc.icon, tc.categories))
		})
	}
}

func TestValidCategories(t *testing.T) {
	assert.Equal(t, []string{"C", "D"}, validCategories([]string{"A;B", "", "C", "x\ny", "D", "E=F"}))
}

func TestExposer_Place(t *testing.T) {
	f := newFixture(t)
	wd := f.Workdir(t, mocks.NsA)
	target := filepath.Join(wd, "Tool.AppImage")
	mocks.WriteFile(t, target, "x", 0o644)
	dollar := filepath.Join(wd, "$Tool")
	mocks.WriteFile(t, dollar, "x", 0o755)
	l := f.Layout(t)
	req := models.Request{Layout: l, Bare: mocks.BareA, Workdir: wd}
	entry := domain.ExposeEntry{Name: "Tool", Icon: "${INSTALL_PATH}/icon.png", Categories: []string{"Development"}}
	loc := filepath.Join(entriesDir(f.UserHome), fileName(mocks.BareA, "Tool"))

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

			got, err := f.exposer.Place(context.Background(), req, entry, models.Candidate{Name: "Tool", Target: tc.target})

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
			if tc.want.Location == "" {
				return
			}
			data, err := os.ReadFile(loc)
			require.NoError(t, err)
			assert.Equal(t, content(mocks.BareA, wd, "Tool", target, filepath.Join(wd, "icon.png"), []string{"Development"}), string(data))
		})
	}
}

func TestExposer_Place_EntryAndDirAreWorldReadable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes are not represented on windows")
	}
	f := newFixture(t)
	wd := f.Workdir(t, mocks.NsA)
	target := filepath.Join(wd, "Tool.AppImage")
	mocks.WriteFile(t, target, "x", 0o644)
	req := models.Request{Layout: f.Layout(t), Bare: mocks.BareA, Workdir: wd}

	got, err := f.exposer.Place(context.Background(), req, domain.ExposeEntry{Name: "Tool"}, models.Candidate{Name: "Tool", Target: target})

	require.NoError(t, err)
	fileInfo, err := os.Stat(got.Location)
	require.NoError(t, err)
	dirInfo, err := os.Stat(entriesDir(f.UserHome))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o044), fileInfo.Mode().Perm()&0o044)
	assert.Equal(t, os.FileMode(0o055), dirInfo.Mode().Perm()&0o055)
}

func TestExposer_Place_MarksAppImageExecutable(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newFixture(t)
	wd := f.Workdir(t, mocks.NsA)
	target := filepath.Join(wd, "Tool.AppImage")
	mocks.WriteFile(t, target, "x", 0o644)
	plain := filepath.Join(wd, "tool")
	mocks.WriteFile(t, plain, "x", 0o644)
	l := f.Layout(t)
	req := models.Request{Layout: l, Bare: mocks.BareA, Workdir: wd}

	_, err := f.exposer.Place(context.Background(), req, domain.ExposeEntry{}, models.Candidate{Name: "Tool", Target: target})
	require.NoError(t, err)
	_, err = f.exposer.Place(context.Background(), req, domain.ExposeEntry{}, models.Candidate{Name: "tool", Target: plain})
	require.NoError(t, err)

	info, err := os.Stat(target)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
	info, err = os.Stat(plain)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())
}

func TestExposer_Place_Errors(t *testing.T) {
	f := newFixture(t)
	wd := f.Workdir(t, mocks.NsA)
	target := filepath.Join(wd, "tool")
	mocks.WriteFile(t, target, "x", 0o755)
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	_, err := f.exposer.Place(context.Background(), models.Request{Layout: models.Layout{UserHome: file}, Bare: mocks.BareA, Workdir: wd}, domain.ExposeEntry{}, models.Candidate{Name: "tool", Target: target})
	require.Error(t, err)

	mocks.RequireUnixHost(t)
	if os.Geteuid() == 0 {
		return
	}
	home := t.TempDir()
	mocks.WriteFile(t, filepath.Join(entriesDir(home), fileName(mocks.BareA, "tool")), "x", 0o000)

	_, err = f.exposer.Place(context.Background(), models.Request{Layout: models.Layout{UserHome: home}, Bare: mocks.BareA, Workdir: wd}, domain.ExposeEntry{}, models.Candidate{Name: "tool", Target: target})
	require.Error(t, err)
}

func TestExposer_Place_SwapError(t *testing.T) {
	f := newFixture(t)
	wd := f.Workdir(t, mocks.NsA)
	target := filepath.Join(wd, "tool")
	mocks.WriteFile(t, target, "x", 0o755)
	l := f.Layout(t)
	loc := filepath.Join(entriesDir(f.UserHome), fileName(mocks.BareA, "tool"))
	mocks.WriteFile(t, filepath.Join(loc+fsguard.StagedSuffix, "blocker"), "x", 0o600)

	_, err := f.exposer.Place(context.Background(), models.Request{Layout: l, Bare: mocks.BareA, Workdir: wd}, domain.ExposeEntry{}, models.Candidate{Name: "tool", Target: target})

	require.Error(t, err)
}

func TestExposer_Remove(t *testing.T) {
	f := newFixture(t)
	dir := entriesDir(f.UserHome)
	own := filepath.Join(dir, fileName(mocks.BareA, "Tool"))
	foreign := filepath.Join(dir, fileName(mocks.BareB, "Tool"))
	user := filepath.Join(dir, "user.desktop")
	mocks.WriteFile(t, own, content(mocks.BareA, "/wd", "Tool", "/x", "", nil), 0o600)
	mocks.WriteFile(t, foreign, content(mocks.BareB, "/wd", "Tool", "/x", "", nil), 0o600)
	mocks.WriteFile(t, user, content(mocks.BareA, "/wd", "Tool", "/x", "", nil), 0o600)
	kept := filepath.Join(dir, fileName(mocks.BareA, "Kept"))
	mocks.WriteFile(t, kept, content(mocks.BareA, "/wd", "Kept", "/x", "", nil), 0o600)

	require.NoError(t, removeEntries(models.Layout{UserHome: f.UserHome}, models.NamespaceClaim(mocks.BareA), map[string]bool{kept: true}))

	assert.NoFileExists(t, own)
	assert.FileExists(t, foreign)
	assert.FileExists(t, user)
	assert.FileExists(t, kept)
}

func TestExposer_Remove_Errors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	blocked := t.TempDir()
	mocks.WriteFile(t, filepath.Join(blocked, ".local", "share", "applications"), "", 0o600)

	assert.NoError(t, removeEntries(models.Layout{UserHome: missing}, models.NamespaceClaim(mocks.BareA), nil))
	assert.Error(t, removeEntries(models.Layout{UserHome: blocked}, models.NamespaceClaim(mocks.BareA), nil))

	mocks.RequireUnixHost(t)
	if os.Geteuid() == 0 {
		return
	}
	home := t.TempDir()
	unreadable := filepath.Join(entriesDir(home), fileName(mocks.BareA, "x"))
	mocks.WriteFile(t, unreadable, content(mocks.BareA, "/wd", "x", "/x", "", nil), 0o000)

	assert.Error(t, removeEntries(models.Layout{UserHome: home}, models.NamespaceClaim(mocks.BareA), nil))

	require.NoError(t, os.Chmod(unreadable, 0o600))
	require.NoError(t, os.Chmod(entriesDir(home), 0o500))
	t.Cleanup(func() { _ = os.Chmod(entriesDir(home), 0o750) })

	assert.Error(t, removeEntries(models.Layout{UserHome: home}, models.NamespaceClaim(mocks.BareA), nil))
}

func TestExposer_Place_CreateDirError(t *testing.T) {
	f := newFixture(t)
	wd := f.Workdir(t, mocks.NsA)
	target := filepath.Join(wd, "tool")
	mocks.WriteFile(t, target, "x", 0o755)
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	req := models.Request{Layout: models.Layout{UserHome: file}, Bare: mocks.BareA, Workdir: wd}

	_, err := f.exposer.Place(context.Background(), req, domain.ExposeEntry{}, models.Candidate{Name: "tool", Target: target})

	require.Error(t, err)
}

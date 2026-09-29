package shelf

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/fsguard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/pathenv"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/platform"
)

type fixture struct {
	*mocks.Sandbox
	shelf *shelf
}

func newFixture(
	t *testing.T,
	goos string,
) *fixture {
	t.Helper()
	sb := mocks.NewSandbox(t, goos)
	return &fixture{
		Sandbox: sb,
		shelf:   newShelf(sb.Host(), sb.Tagger, sb.UserPath),
	}
}

func homeAt(
	dir string,
) Shelf {
	host := platform.NewHost()
	host.HomeDir = dir
	return newShelf(host, &mocks.Tagger{}, &mocks.UserPath{})
}

func lnkDir(
	appData string,
) string {
	return filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs", "Quiver")
}

func xdgDir(
	userHome string,
) string {
	return filepath.Join(userHome, ".local", "share", "applications")
}

func cliExpose(
	name string,
	path string,
) domain.Expose {
	return domain.Expose{CLI: []domain.ExposeEntry{{Name: name, Path: path}}}
}

func TestNew_Defaults(t *testing.T) {
	s := New().(*shelf)

	assert.Equal(t, runtime.GOOS, s.host.GOOS)
	assert.Equal(t, runtime.GOARCH, s.host.GOARCH)
	assert.Equal(t, pathenv.NewUserPath(), s.userPath)
	assert.Empty(t, s.host.HomeDir)
	assert.Nil(t, s.host.AppsDirs)
	assert.Equal(t, "/h", New(WithSandboxHome("/h")).(*shelf).host.SandboxHome)
}

func TestShelf_Apply_EmptyExposeStillValidates(t *testing.T) {
	f := newFixture(t, "linux")

	_, err := f.shelf.Apply(context.Background(), mocks.NsA, "relative", domain.Expose{}, domain.ArrowMedia{})

	require.Error(t, err)
}

func TestShelf_Apply_PrunesStaleOwnedEntries(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newFixture(t, "linux")
	wd1 := f.Workdir(t, mocks.NsA)
	wd2 := f.Workdir(t, mocks.NsA2)
	wdB := f.Workdir(t, mocks.NsB)
	for _, wd := range []string{wd1, wd2} {
		mocks.WriteFile(t, filepath.Join(wd, "keep"), "x", 0o755)
		mocks.WriteFile(t, filepath.Join(wd, "old"), "x", 0o755)
		mocks.WriteFile(t, filepath.Join(wd, "Tool.AppImage"), "x", 0o755)
	}
	mocks.WriteFile(t, filepath.Join(wdB, "thing"), "x", 0o755)
	mocks.WriteFile(t, filepath.Join(f.Bin, "mine"), "user", 0o755)
	v1 := domain.Expose{
		CLI:     []domain.ExposeEntry{{Name: "keep", Path: "keep"}, {Name: "old", Path: "old"}},
		Desktop: []domain.ExposeEntry{{Name: "Tool", Path: "Tool.AppImage"}},
	}
	v2 := domain.Expose{CLI: []domain.ExposeEntry{{Name: "keep", Path: "keep"}}}

	first, err := f.shelf.Apply(context.Background(), mocks.NsA, wd1, v1, domain.ArrowMedia{})
	require.NoError(t, err)
	require.Len(t, first.Entries, 3)
	_, err = f.shelf.Apply(context.Background(), mocks.NsB, wdB, cliExpose("thing", "thing"), domain.ArrowMedia{})
	require.NoError(t, err)
	second, err := f.shelf.Apply(context.Background(), mocks.NsA2, wd2, v2, domain.ArrowMedia{})
	require.NoError(t, err)

	require.Len(t, second.Entries, 1)
	link, err := os.Readlink(filepath.Join(f.Bin, "keep"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(wd2, "keep"), link)
	assert.NoFileExists(t, filepath.Join(f.Bin, "old"))
	assert.NoFileExists(t, first.Entries[0].Location)
	assert.FileExists(t, filepath.Join(f.Bin, "thing"))
	assert.FileExists(t, filepath.Join(f.Bin, "mine"))

	_, err = f.shelf.Apply(context.Background(), mocks.NsA2, wd2, domain.Expose{}, domain.ArrowMedia{})
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(f.Bin, "keep"))
	assert.FileExists(t, filepath.Join(f.Bin, "thing"))
}

func TestShelf_Apply_PruneError_ReturnsError(t *testing.T) {
	f := newFixture(t, platform.GOOSWindows)
	wd := f.Workdir(t, mocks.NsA)
	mocks.WriteFile(t, filepath.Join(lnkDir(filepath.Join(f.UserHome, "AppData", "Roaming")), "stale.lnk"), "", 0o600)
	errBoom := errors.New("boom")
	f.Cmd.Respond = func(string, []string, []string) ([]byte, error) { return nil, errBoom }

	_, err := f.shelf.Apply(context.Background(), mocks.NsA, wd, domain.Expose{}, domain.ArrowMedia{})

	require.ErrorIs(t, err, errBoom)
}

func TestShelf_Apply_InvalidRequest_ReturnsError(t *testing.T) {
	f := newFixture(t, "linux")
	wdB := f.Workdir(t, mocks.NsB)

	testCases := []struct {
		name    string
		ns      domain.Namespace
		workdir string
	}{
		{
			name:    "invalid namespace",
			ns:      "nope",
			workdir: wdB,
		},
		{
			name:    "workdir of another namespace",
			ns:      mocks.NsA,
			workdir: wdB,
		},
		{
			name:    "workdir outside namespaces",
			ns:      mocks.NsA,
			workdir: t.TempDir(),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := f.shelf.Apply(context.Background(), tc.ns, tc.workdir, cliExpose("tool", "tool"), domain.ArrowMedia{})
			require.Error(t, err)
			assert.Zero(t, got)
		})
	}
}

func TestShelf_Apply_LayoutError_ReturnsError(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	s := homeAt(file)

	got, err := s.Apply(context.Background(), mocks.NsA, file, cliExpose("tool", "tool"), domain.ArrowMedia{})

	require.Error(t, err)
	assert.Zero(t, got)
}

func TestShelf_Apply_CLISymlinkCreated(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newFixture(t, "linux")
	wd := f.Workdir(t, mocks.NsA)
	target := filepath.Join(wd, "bin", "tool")
	mocks.WriteFile(t, target, "#!/bin/sh\n", 0o755)

	got, err := f.shelf.Apply(context.Background(), mocks.NsA, wd, cliExpose("tool", "${INSTALL_PATH}/bin/tool"), domain.ArrowMedia{})

	require.NoError(t, err)
	assert.Empty(t, got.Refused)
	assert.Equal(t, []AppliedEntry{{
		Kind:     domain.ExposeKindCLI,
		Name:     "tool",
		Target:   target,
		Location: filepath.Join(f.Bin, "tool"),
	}}, got.Entries)
	link, err := os.Readlink(filepath.Join(f.Bin, "tool"))
	require.NoError(t, err)
	assert.Equal(t, target, link)
}

func TestShelf_Apply_ResolveRefusalRecorded(t *testing.T) {
	f := newFixture(t, "linux")
	wd := f.Workdir(t, mocks.NsA)

	got, err := f.shelf.Apply(context.Background(), mocks.NsA, wd, cliExpose("../x", "tool"), domain.ArrowMedia{})
	require.NoError(t, err)

	assert.Equal(t, []Refusal{{Kind: domain.ExposeKindCLI, Name: "../x", Reason: models.ReasonUnsafeName}}, got.Refused)
}

func TestShelf_Apply_ResolveError_ReturnsError(t *testing.T) {
	f := newFixture(t, "linux")
	wd := f.Workdir(t, mocks.NsA)
	require.NoError(t, os.RemoveAll(wd))

	_, err := f.shelf.Apply(context.Background(), mocks.NsA, wd, cliExpose("tool", domain.ExposeAuto), domain.ArrowMedia{})

	require.ErrorIs(t, err, fs.ErrNotExist)
}

func TestShelf_Apply_PlaceError_ReturnsError(t *testing.T) {
	errBoom := errors.New("boom")

	testCases := []struct {
		name   string
		expose func(wd string) domain.Expose
		check  func(t *testing.T, f *fixture, err error)
	}{
		{
			name: "cli",
			expose: func(string) domain.Expose {
				return cliExpose("tool", "tool")
			},
			check: func(t *testing.T, f *fixture, err error) {
				require.Error(t, err)
				assert.FileExists(t, filepath.Join(f.Bin, "tool.cmd"+fsguard.StagedSuffix, "x"))
				assert.NoFileExists(t, filepath.Join(f.Bin, "tool.cmd"))
			},
		},
		{
			name: "desktop",
			expose: func(string) domain.Expose {
				return domain.Expose{Desktop: []domain.ExposeEntry{{Name: "tool", Path: "tool"}}}
			},
			check: func(t *testing.T, _ *fixture, err error) {
				require.ErrorIs(t, err, errBoom)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, platform.GOOSWindows)
			wd := f.Workdir(t, mocks.NsA)
			mocks.WriteFile(t, filepath.Join(wd, "tool"), "x", 0o755)
			mocks.WriteFile(t, filepath.Join(f.Bin, "tool.cmd"+fsguard.StagedSuffix, "x"), "", 0o600)
			f.Cmd.Respond = func(string, []string, []string) ([]byte, error) { return nil, errBoom }
			mocks.WriteFile(t, filepath.Join(lnkDir(filepath.Join(f.UserHome, "AppData", "Roaming")), "tool.lnk"), "", 0o600)

			_, err := f.shelf.Apply(context.Background(), mocks.NsA, wd, tc.expose(wd), domain.ArrowMedia{})

			tc.check(t, f, err)
		})
	}
}

func TestShelf_Place_UnknownKind_ReturnsError(t *testing.T) {
	f := newFixture(t, "linux")

	got, err := f.shelf.place(context.Background(), models.ApplyRequest{}, "bogus", domain.ExposeEntry{}, models.Candidate{})

	require.Error(t, err)
	assert.Zero(t, got)
}

func TestShelf_Apply_DesktopBundleRelocatesCLITargets(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newFixture(t, platform.GOOSDarwin)
	wd := f.Workdir(t, mocks.NsA)
	mocks.WriteFile(t, filepath.Join(wd, "Tool.app", "Contents", "MacOS", "tool"), "x", 0o755)
	expose := domain.Expose{
		CLI:     []domain.ExposeEntry{{Name: "tool", Path: "${INSTALL_PATH}/Tool.app/Contents/MacOS/tool"}},
		Desktop: []domain.ExposeEntry{{Name: "Tool", Path: "${INSTALL_PATH}/Tool.app"}},
	}
	bundle := filepath.Join(f.Apps[0], "Tool.app")
	inBundle := filepath.Join(bundle, "Contents", "MacOS", "tool")

	first, err := f.shelf.Apply(context.Background(), mocks.NsA, wd, expose, domain.ArrowMedia{})
	require.NoError(t, err)
	second, err := f.shelf.Apply(context.Background(), mocks.NsA, wd, expose, domain.ArrowMedia{})
	require.NoError(t, err)

	assert.Empty(t, first.Refused)
	assert.Empty(t, second.Refused)
	assert.Equal(t, first, second)
	require.Len(t, first.Entries, 2)
	assert.Equal(t, domain.ExposeKindDesktop, first.Entries[0].Kind)
	assert.Equal(t, bundle, first.Entries[0].Location)
	assert.Equal(t, inBundle, first.Entries[1].Target)
	link, err := os.Readlink(filepath.Join(f.Bin, "tool"))
	require.NoError(t, err)
	assert.Equal(t, inBundle, link)

	require.NoError(t, f.shelf.Remove(context.Background(), wd))
	assert.NoFileExists(t, filepath.Join(f.Bin, "tool"))
	assert.NoDirExists(t, bundle)
}

func TestShelf_Remove_CleansOnlyOwnEntries(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newFixture(t, "linux")
	wdA := f.Workdir(t, mocks.NsA)
	wdB := f.Workdir(t, mocks.NsB)
	mocks.WriteFile(t, filepath.Join(wdA, "tool"), "a", 0o755)
	mocks.WriteFile(t, filepath.Join(wdA, "Tool.AppImage"), "a", 0o755)
	mocks.WriteFile(t, filepath.Join(wdB, "thing"), "b", 0o755)
	mocks.WriteFile(t, filepath.Join(f.Bin, "mine"), "user", 0o755)
	userDesktop := filepath.Join(xdgDir(f.UserHome), "user.desktop")
	mocks.WriteFile(t, userDesktop, "[Desktop Entry]\n", 0o600)

	exposeA := domain.Expose{
		CLI:     []domain.ExposeEntry{{Name: "tool", Path: "tool"}},
		Desktop: []domain.ExposeEntry{{Name: "Tool", Path: "Tool.AppImage"}},
	}
	gotA, err := f.shelf.Apply(context.Background(), mocks.NsA, wdA, exposeA, domain.ArrowMedia{})
	require.NoError(t, err)
	require.Len(t, gotA.Entries, 2)
	_, err = f.shelf.Apply(context.Background(), mocks.NsB, wdB, cliExpose("thing", "thing"), domain.ArrowMedia{})
	require.NoError(t, err)

	require.NoError(t, f.shelf.Remove(context.Background(), wdA))

	for _, e := range gotA.Entries {
		assert.NoFileExists(t, e.Location)
	}
	assert.FileExists(t, filepath.Join(f.Bin, "thing"))
	assert.FileExists(t, filepath.Join(f.Bin, "mine"))
	assert.FileExists(t, userDesktop)
}

func TestShelf_Remove_InvalidRequest_RemovesNothing(t *testing.T) {
	testCases := []struct {
		name  string
		setup func(t *testing.T) (Shelf, string, []string)
	}{
		{
			name: "not a namespace workdir",
			setup: func(t *testing.T) (Shelf, string, []string) {
				mocks.RequireUnixHost(t)
				f := newFixture(t, "linux")
				wd := f.Workdir(t, mocks.NsA)
				mocks.WriteFile(t, filepath.Join(wd, "tool"), "a", 0o755)
				applied, err := f.shelf.Apply(context.Background(), mocks.NsA, wd, cliExpose("tool", "tool"), domain.ArrowMedia{})
				require.NoError(t, err)
				require.NotEmpty(t, applied.Entries)
				return f.shelf, f.NsDir, []string{applied.Entries[0].Location}
			},
		},
		{
			name: "layout error",
			setup: func(t *testing.T) (Shelf, string, []string) {
				file := filepath.Join(t.TempDir(), "file")
				require.NoError(t, os.WriteFile(file, nil, 0o600))
				return homeAt(file), file, []string{file}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			shelf, workdir, survivors := tc.setup(t)

			err := shelf.Remove(context.Background(), workdir)

			require.Error(t, err)
			for _, path := range survivors {
				_, statErr := os.Lstat(path)
				assert.NoError(t, statErr, path)
			}
		})
	}
}

func TestShelf_Remove_JoinsErrors(t *testing.T) {
	f := newFixture(t, platform.GOOSWindows)
	mocks.WriteFile(t, filepath.Join(lnkDir(filepath.Join(f.UserHome, "AppData", "Roaming")), "tool.lnk"), "", 0o600)
	errBoom := errors.New("boom")
	f.Cmd.Respond = func(string, []string, []string) ([]byte, error) { return nil, errBoom }

	err := f.shelf.Remove(context.Background(), f.Workdir(t, mocks.NsA))

	require.ErrorIs(t, err, errBoom)
}

func TestShelf_PathMethods_Delegate(t *testing.T) {
	f := newFixture(t, "linux")
	f.Env["SHELL"] = "/bin/zsh"

	status, err := f.shelf.PathStatus(context.Background())
	require.NoError(t, err)
	assert.False(t, status.Configured)

	status, err = f.shelf.SetupPath(context.Background())
	require.NoError(t, err)
	assert.True(t, status.Configured)
	assert.Equal(t, f.Bin, status.BinDir)
}

func TestShelf_Apply_AutoEntryThatResolvesToNothingIsSkipped(t *testing.T) {
	testCases := []struct {
		name        string
		goos        string
		file        string
		expose      domain.Expose
		wantRefused []Refusal
	}{
		{
			name:   "auto desktop on darwin without a bundle",
			goos:   platform.GOOSDarwin,
			expose: domain.Expose{Desktop: []domain.ExposeEntry{{Name: "tool", Path: domain.ExposeAuto}}},
		},
		{
			name:   "auto desktop on linux without an appimage",
			goos:   "linux",
			expose: domain.Expose{Desktop: []domain.ExposeEntry{{Name: "tool", Path: domain.ExposeAuto}}},
		},
		{
			name:   "auto cli without an executable",
			goos:   "linux",
			expose: cliExpose("tool", domain.ExposeAuto),
		},
		{
			name:   "auto cli named after a file without the exec bit",
			goos:   "linux",
			file:   "tool",
			expose: cliExpose("tool", domain.ExposeAuto),
		},
		{
			name:        "declared desktop that does not exist",
			goos:        platform.GOOSDarwin,
			expose:      domain.Expose{Desktop: []domain.ExposeEntry{{Name: "Tool", Path: "${INSTALL_PATH}/Tool.app"}}},
			wantRefused: []Refusal{{Kind: domain.ExposeKindDesktop, Name: "Tool", Reason: models.ReasonNotFound}},
		},
		{
			name:        "declared cli that does not exist",
			goos:        "linux",
			expose:      cliExpose("tool", "${INSTALL_PATH}/tool"),
			wantRefused: []Refusal{{Kind: domain.ExposeKindCLI, Name: "tool", Reason: models.ReasonNotFound}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, tc.goos)
			wd := f.Workdir(t, mocks.NsA)
			mocks.WriteFile(t, filepath.Join(wd, "README"), "x", 0o644)
			if tc.file != "" {
				mocks.RequireUnixHost(t)
				mocks.WriteFile(t, filepath.Join(wd, tc.file), "#!/bin/sh\n", 0o644)
			}

			got, err := f.shelf.Apply(context.Background(), mocks.NsA, wd, tc.expose, domain.ArrowMedia{})

			require.NoError(t, err)
			assert.Empty(t, got.Entries)
			assert.Equal(t, tc.wantRefused, got.Refused)
		})
	}
}

func TestWithSandboxHome_ResolvesNothingOutsideHome(t *testing.T) {
	testCases := []struct {
		name string
		goos string
	}{
		{name: "darwin", goos: platform.GOOSDarwin},
		{name: "linux", goos: "linux"},
		{name: "windows", goos: platform.GOOSWindows},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			host := platform.NewHost()
			host.GOOS = tc.goos
			host.Env = func(key string) string { return "/real/" + key }
			WithSandboxHome(home)(&host)
			s := newShelf(host, &mocks.Tagger{}, &mocks.UserPath{})

			l, err := s.host.Layout()
			require.NoError(t, err)

			dirs := append([]string{
				l.Bin,
				l.Namespaces,
				l.UserHome,
				xdgDir(l.UserHome),
				lnkDir(s.host.AppData(l.UserHome)),
			}, l.Apps...)
			for _, dir := range dirs {
				rel, err := filepath.Rel(home, dir)
				require.NoError(t, err)
				assert.False(t, strings.HasPrefix(rel, ".."), "%s escapes %s", dir, home)
			}
			assert.Equal(t, []string{filepath.Join(home, "Applications")}, l.Apps)
		})
	}
}

func TestShelf_Remove_SparesEntriesTakenOverBySiblingRef(t *testing.T) {
	mocks.RequireUnixHost(t)
	testCases := []struct {
		name    string
		goos    string
		expose  domain.Expose
		entries int
	}{
		{
			name:    "cli symlink and xdg entry",
			goos:    "linux",
			expose:  domain.Expose{CLI: []domain.ExposeEntry{{Name: "tool", Path: "tool"}}, Desktop: []domain.ExposeEntry{{Name: "Tool", Path: "Tool.AppImage"}}},
			entries: 2,
		},
		{
			name:    "cli shim",
			goos:    platform.GOOSWindows,
			expose:  cliExpose("tool", "tool"),
			entries: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, tc.goos)
			wd1 := f.Workdir(t, mocks.NsA)
			wd2 := f.Workdir(t, mocks.NsA2)
			for _, wd := range []string{wd1, wd2} {
				mocks.WriteFile(t, filepath.Join(wd, "tool"), "x", 0o755)
				mocks.WriteFile(t, filepath.Join(wd, "Tool.AppImage"), "x", 0o755)
			}
			_, err := f.shelf.Apply(context.Background(), mocks.NsA, wd1, tc.expose, domain.ArrowMedia{})
			require.NoError(t, err)
			v2, err := f.shelf.Apply(context.Background(), mocks.NsA2, wd2, tc.expose, domain.ArrowMedia{})
			require.NoError(t, err)
			require.Len(t, v2.Entries, tc.entries)

			require.NoError(t, f.shelf.Remove(context.Background(), wd1))
			for _, e := range v2.Entries {
				_, statErr := os.Lstat(e.Location)
				assert.NoError(t, statErr, e.Location)
			}

			require.NoError(t, f.shelf.Remove(context.Background(), wd2))
			for _, e := range v2.Entries {
				_, statErr := os.Lstat(e.Location)
				assert.ErrorIs(t, statErr, fs.ErrNotExist, e.Location)
			}
		})
	}
}

func TestShelf_Remove_SweepsEntriesOfAVanishedWorkdir(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newFixture(t, "linux")
	wd1 := f.Workdir(t, mocks.NsA)
	wd2 := f.Workdir(t, mocks.NsA2)
	mocks.WriteFile(t, filepath.Join(wd1, "old"), "x", 0o755)
	applied, err := f.shelf.Apply(context.Background(), mocks.NsA, wd1, cliExpose("old", "old"), domain.ArrowMedia{})
	require.NoError(t, err)
	require.Len(t, applied.Entries, 1)
	require.NoError(t, os.RemoveAll(wd1))

	require.NoError(t, f.shelf.Remove(context.Background(), wd2))

	_, statErr := os.Lstat(applied.Entries[0].Location)
	assert.ErrorIs(t, statErr, fs.ErrNotExist)
}

func TestShelf_Remove_OutsideNamespacesIsNotAWorkdir(t *testing.T) {
	f := newFixture(t, "linux")

	err := f.shelf.Remove(context.Background(), t.TempDir())

	require.ErrorIs(t, err, ErrNotAWorkdir)
}

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
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/fsguard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/ownership"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/pathenv"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/platform"
)

type fixture struct {
	*mocks.Sandbox
	shelf *shelf
}

func newFixture(
	t *testing.T,
	goos string,
	opts ...Option,
) *fixture {
	t.Helper()
	sb := mocks.NewSandbox(t, goos)
	base := []Option{
		WithHomeDir(sb.Home),
		WithUserHomeDir(sb.UserHome),
		WithAppsDirs(sb.Apps),
		WithGOOS(goos),
		WithGOARCH("amd64"),
		WithCommander(sb.Cmd),
		WithEnv(sb.Lookup),
		withTagger(sb.Tagger),
		withUserPath(sb.UserPath),
	}
	return &fixture{
		Sandbox: sb,
		shelf:   New(append(base, opts...)...).(*shelf),
	}
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
	assert.Equal(t, platform.NewCommander(), s.host.Commander)
	assert.Equal(t, ownership.NewTagger(), s.tagger)
	assert.Equal(t, pathenv.NewUserPath(), s.userPath)
	assert.NotNil(t, s.host.Env)
	assert.Empty(t, s.host.HomeDir)
	assert.Empty(t, s.host.UserHomeDir)
	assert.Nil(t, s.host.AppsDirs)
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

func TestShelf_Apply_SymlinkEscapesRefused(t *testing.T) {
	mocks.RequireUnixHost(t)
	machO := string([]byte{0xcf, 0xfa, 0xed, 0xfe})

	testCases := []struct {
		name    string
		goos    string
		outside string
		link    string
		expose  domain.Expose
		check   func(t *testing.T, f *fixture, outside string)
	}{
		{
			name:    "bundle through a symlinked parent",
			goos:    platform.GOOSDarwin,
			outside: "User.app/Contents/version",
			link:    "lib",
			expose:  domain.Expose{Desktop: []domain.ExposeEntry{{Name: "User", Path: "${INSTALL_PATH}/lib/User.app"}}},
			check: func(t *testing.T, f *fixture, outside string) {
				assert.DirExists(t, filepath.Join(outside, "User.app"))
				assert.NoFileExists(t, filepath.Join(outside, "User.app", mocks.StubTagKey))
				assert.NoDirExists(t, filepath.Join(f.Apps[0], "User.app"))
			},
		},
		{
			name:    "appimage through a symlinked parent",
			goos:    "linux",
			outside: "X.AppImage",
			link:    "lib",
			expose:  domain.Expose{Desktop: []domain.ExposeEntry{{Name: "X", Path: "${INSTALL_PATH}/lib/X.AppImage"}}},
			check: func(t *testing.T, f *fixture, outside string) {
				info, err := os.Stat(filepath.Join(outside, "X.AppImage"))
				require.NoError(t, err)
				assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())
			},
		},
		{
			name:    "codesign target through a symlinked parent",
			goos:    platform.GOOSDarwin,
			outside: "tool",
			link:    "bin",
			expose:  cliExpose("tool", "${INSTALL_PATH}/bin/tool"),
			check: func(t *testing.T, f *fixture, _ string) {
				assert.Empty(t, f.Cmd.Calls)
				assert.NoFileExists(t, filepath.Join(f.Bin, "tool"))
			},
		},
		{
			name:    "final symlink leaving the workdir",
			goos:    "linux",
			outside: "tool",
			link:    "tool",
			expose:  cliExpose("tool", "${INSTALL_PATH}/tool"),
			check: func(t *testing.T, f *fixture, _ string) {
				assert.NoFileExists(t, filepath.Join(f.Bin, "tool"))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, tc.goos, WithGOARCH(platform.GOARCHARM64))
			wd := f.Workdir(t, mocks.NsA)
			outside := t.TempDir()
			mocks.WriteFile(t, filepath.Join(outside, filepath.FromSlash(tc.outside)), machO, 0o644)
			linkTarget := outside
			if tc.link == "tool" {
				linkTarget = filepath.Join(outside, "tool")
			}
			require.NoError(t, os.Symlink(linkTarget, filepath.Join(wd, tc.link)))

			got, err := f.shelf.Apply(context.Background(), mocks.NsA, wd, tc.expose, domain.ArrowMedia{})

			require.NoError(t, err)
			assert.Empty(t, got.Entries)
			require.Len(t, got.Refused, 1)
			assert.Equal(t, models.ReasonOutsideWorkdir, got.Refused[0].Reason)
			tc.check(t, f, outside)
		})
	}
}

func TestShelf_Apply_SymlinkedBundleInsideWorkdirRefused(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newFixture(t, platform.GOOSDarwin)
	wd := f.Workdir(t, mocks.NsA)
	mocks.WriteBundle(t, filepath.Join(wd, "real", "Tool.app"), "v1")
	require.NoError(t, os.Symlink(filepath.Join(wd, "real", "Tool.app"), filepath.Join(wd, "Tool.app")))

	got, err := f.shelf.Apply(context.Background(), mocks.NsA, wd, domain.Expose{Desktop: []domain.ExposeEntry{{Name: "Tool", Path: "Tool.app"}}}, domain.ArrowMedia{})

	require.NoError(t, err)
	assert.Equal(t, []Refusal{{Kind: domain.ExposeKindDesktop, Name: "Tool", Reason: models.ReasonWrongType}}, got.Refused)
	assert.DirExists(t, filepath.Join(wd, "real", "Tool.app"))
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
	s := New(WithHomeDir(file))

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

func TestShelf_Apply_DeclaredCLIMadeExecutable(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newFixture(t, "linux")
	wd := f.Workdir(t, mocks.NsA)
	target := filepath.Join(wd, "tool")
	mocks.WriteFile(t, target, "#!/bin/sh\n", 0o644)

	got, err := f.shelf.Apply(context.Background(), mocks.NsA, wd, cliExpose("tool", "${INSTALL_PATH}/tool"), domain.ArrowMedia{})

	require.NoError(t, err)
	assert.Empty(t, got.Refused)
	info, err := os.Stat(target)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
	link, err := os.Readlink(filepath.Join(f.Bin, "tool"))
	require.NoError(t, err)
	assert.Equal(t, target, link)
}

func TestShelf_Apply_DeclaredCLIOutsideWorkdirNotMarked(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newFixture(t, "linux")
	wd := f.Workdir(t, mocks.NsA)
	outside := filepath.Join(t.TempDir(), "tool")
	mocks.WriteFile(t, outside, "#!/bin/sh\n", 0o644)
	require.NoError(t, os.Symlink(outside, filepath.Join(wd, "tool")))

	got, err := f.shelf.Apply(context.Background(), mocks.NsA, wd, cliExpose("tool", "${INSTALL_PATH}/tool"), domain.ArrowMedia{})

	require.NoError(t, err)
	require.Len(t, got.Refused, 1)
	assert.Equal(t, models.ReasonOutsideWorkdir, got.Refused[0].Reason)
	info, err := os.Stat(outside)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())
}

func TestShelf_Apply_AutoCLIStillNeedsExecBit(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newFixture(t, "linux")
	wd := f.Workdir(t, mocks.NsA)
	target := filepath.Join(wd, "tool")
	mocks.WriteFile(t, target, "#!/bin/sh\n", 0o644)

	got, err := f.shelf.Apply(context.Background(), mocks.NsA, wd, cliExpose("tool", domain.ExposeAuto), domain.ArrowMedia{})

	require.NoError(t, err)
	assert.Empty(t, got.Refused)
	assert.Empty(t, got.Entries)
	info, err := os.Stat(target)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())
}

func TestShelf_Apply_ReapplyNewWorkdirRepoints(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newFixture(t, "linux")
	wd1 := f.Workdir(t, mocks.NsA)
	wd2 := f.Workdir(t, mocks.NsA2)
	mocks.WriteFile(t, filepath.Join(wd1, "tool"), "v1", 0o755)
	mocks.WriteFile(t, filepath.Join(wd2, "tool"), "v2", 0o755)

	_, err := f.shelf.Apply(context.Background(), mocks.NsA, wd1, cliExpose("tool", "${WORKDIR}/tool"), domain.ArrowMedia{})
	require.NoError(t, err)
	got, err := f.shelf.Apply(context.Background(), mocks.NsA2, wd2, cliExpose("tool", "${WORKDIR}/tool"), domain.ArrowMedia{})
	require.NoError(t, err)

	assert.Empty(t, got.Refused)
	require.Len(t, got.Entries, 1)
	link, err := os.Readlink(filepath.Join(f.Bin, "tool"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(wd2, "tool"), link)
}

func TestShelf_Apply_IsIdempotent(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newFixture(t, "linux")
	wd := f.Workdir(t, mocks.NsA)
	mocks.WriteFile(t, filepath.Join(wd, "tool"), "x", 0o755)
	expose := cliExpose("tool", "tool")

	first, err := f.shelf.Apply(context.Background(), mocks.NsA, wd, expose, domain.ArrowMedia{})
	require.NoError(t, err)
	second, err := f.shelf.Apply(context.Background(), mocks.NsA, wd, expose, domain.ArrowMedia{})
	require.NoError(t, err)

	assert.Equal(t, first, second)
	assert.Empty(t, second.Refused)
}

func TestShelf_Apply_ForeignOwnerRefused(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newFixture(t, "linux")
	wdA := f.Workdir(t, mocks.NsA)
	wdB := f.Workdir(t, mocks.NsB)
	mocks.WriteFile(t, filepath.Join(wdA, "tool"), "a", 0o755)
	mocks.WriteFile(t, filepath.Join(wdB, "tool"), "b", 0o755)

	_, err := f.shelf.Apply(context.Background(), mocks.NsB, wdB, cliExpose("tool", "tool"), domain.ArrowMedia{})
	require.NoError(t, err)
	got, err := f.shelf.Apply(context.Background(), mocks.NsA, wdA, cliExpose("tool", "tool"), domain.ArrowMedia{})
	require.NoError(t, err)

	assert.Empty(t, got.Entries)
	assert.Equal(t, []Refusal{{
		Kind:   domain.ExposeKindCLI,
		Name:   "tool",
		Reason: "owned by github.com/other/thing",
	}}, got.Refused)
	link, err := os.Readlink(filepath.Join(f.Bin, "tool"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(wdB, "tool"), link)
}

func TestShelf_Apply_UnownedFileRefused(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newFixture(t, "linux")
	wd := f.Workdir(t, mocks.NsA)
	mocks.WriteFile(t, filepath.Join(wd, "tool"), "a", 0o755)
	mocks.WriteFile(t, filepath.Join(f.Bin, "tool"), "mine", 0o755)

	got, err := f.shelf.Apply(context.Background(), mocks.NsA, wd, cliExpose("tool", "tool"), domain.ArrowMedia{})
	require.NoError(t, err)

	assert.Empty(t, got.Entries)
	assert.Equal(t, []Refusal{{Kind: domain.ExposeKindCLI, Name: "tool", Reason: models.ReasonUnmanaged}}, got.Refused)
	data, err := os.ReadFile(filepath.Join(f.Bin, "tool"))
	require.NoError(t, err)
	assert.Equal(t, "mine", string(data))
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

	require.NoError(t, f.shelf.Remove(context.Background(), mocks.NsA))
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

	require.NoError(t, f.shelf.Remove(context.Background(), mocks.NsA2))

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
		setup func(t *testing.T) (Shelf, domain.Namespace, []string)
	}{
		{
			name: "invalid namespace",
			setup: func(t *testing.T) (Shelf, domain.Namespace, []string) {
				mocks.RequireUnixHost(t)
				f := newFixture(t, "linux")
				wd := f.Workdir(t, mocks.NsA)
				mocks.WriteFile(t, filepath.Join(wd, "tool"), "a", 0o755)
				applied, err := f.shelf.Apply(context.Background(), mocks.NsA, wd, cliExpose("tool", "tool"), domain.ArrowMedia{})
				require.NoError(t, err)
				require.NotEmpty(t, applied.Entries)
				return f.shelf, "nope", []string{applied.Entries[0].Location}
			},
		},
		{
			name: "layout error",
			setup: func(t *testing.T) (Shelf, domain.Namespace, []string) {
				file := filepath.Join(t.TempDir(), "file")
				require.NoError(t, os.WriteFile(file, nil, 0o600))
				return New(WithHomeDir(file)), mocks.NsA, []string{file}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			shelf, ns, survivors := tc.setup(t)

			err := shelf.Remove(context.Background(), ns)

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

	err := f.shelf.Remove(context.Background(), mocks.NsA)

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

func TestShelf_Apply_AutoCLINamedAfterTheExecutable(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newFixture(t, "linux")
	wd := f.Workdir(t, mocks.NsA)
	wd2 := f.Workdir(t, mocks.NsA2)
	mocks.WriteFile(t, filepath.Join(wd, "ripgrep-14", "rg"), "x", 0o755)
	mocks.WriteFile(t, filepath.Join(wd2, "fd"), "x", 0o755)

	first, err := f.shelf.Apply(context.Background(), mocks.NsA, wd, cliExpose("ripgrep", domain.ExposeAuto), domain.ArrowMedia{})
	require.NoError(t, err)

	require.Len(t, first.Entries, 1)
	assert.Empty(t, first.Refused)
	assert.Equal(t, "rg", first.Entries[0].Name)
	assert.Equal(t, filepath.Join(f.Bin, "rg"), first.Entries[0].Location)
	link, err := os.Readlink(filepath.Join(f.Bin, "rg"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(wd, "ripgrep-14", "rg"), link)
	assert.NoFileExists(t, filepath.Join(f.Bin, "ripgrep"))

	second, err := f.shelf.Apply(context.Background(), mocks.NsA2, wd2, cliExpose("ripgrep", domain.ExposeAuto), domain.ArrowMedia{})
	require.NoError(t, err)

	require.Len(t, second.Entries, 1)
	assert.Equal(t, "fd", second.Entries[0].Name)
	assert.FileExists(t, filepath.Join(f.Bin, "fd"))
	_, err = os.Lstat(filepath.Join(f.Bin, "rg"))
	assert.ErrorIs(t, err, fs.ErrNotExist)
}

func TestShelf_Apply_AutoCLIForeignOwnerRefusedUnderTheExecutableName(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newFixture(t, "linux")
	wdA := f.Workdir(t, mocks.NsA)
	wdB := f.Workdir(t, mocks.NsB)
	mocks.WriteFile(t, filepath.Join(wdA, "rg"), "a", 0o755)
	mocks.WriteFile(t, filepath.Join(wdB, "rg"), "b", 0o755)

	_, err := f.shelf.Apply(context.Background(), mocks.NsB, wdB, cliExpose("rg", "rg"), domain.ArrowMedia{})
	require.NoError(t, err)
	got, err := f.shelf.Apply(context.Background(), mocks.NsA, wdA, cliExpose("ripgrep", domain.ExposeAuto), domain.ArrowMedia{})
	require.NoError(t, err)

	assert.Empty(t, got.Entries)
	assert.Equal(t, []Refusal{{Kind: domain.ExposeKindCLI, Name: "rg", Reason: "owned by github.com/other/thing"}}, got.Refused)
}

func TestShelf_Apply_AutoCLIOnWindowsStripsExe(t *testing.T) {
	f := newFixture(t, platform.GOOSWindows)
	wd := f.Workdir(t, mocks.NsA)
	mocks.WriteFile(t, filepath.Join(wd, "rg.exe"), "x", 0o755)

	got, err := f.shelf.Apply(context.Background(), mocks.NsA, wd, cliExpose("ripgrep", domain.ExposeAuto), domain.ArrowMedia{})
	require.NoError(t, err)

	require.Len(t, got.Entries, 1)
	assert.Equal(t, "rg", got.Entries[0].Name)
	assert.Equal(t, filepath.Join(f.Bin, "rg.cmd"), got.Entries[0].Location)
	assert.NoFileExists(t, filepath.Join(f.Bin, "ripgrep.cmd"))
}

func TestShelf_Apply_AutoEntryThatResolvesToNothingIsSkipped(t *testing.T) {
	testCases := []struct {
		name        string
		goos        string
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

			got, err := f.shelf.Apply(context.Background(), mocks.NsA, wd, tc.expose, domain.ArrowMedia{})

			require.NoError(t, err)
			assert.Empty(t, got.Entries)
			assert.Equal(t, tc.wantRefused, got.Refused)
		})
	}
}

func TestShelf_Apply_AutoDesktopBundleKeepsItsOwnName(t *testing.T) {
	f := newFixture(t, platform.GOOSDarwin)
	wd := f.Workdir(t, mocks.NsA)
	wd2 := f.Workdir(t, mocks.NsA2)
	mocks.WriteBundle(t, filepath.Join(wd, "CC Switch.app"), "v1")
	mocks.WriteBundle(t, filepath.Join(wd2, "CC Switch.app"), "v2")
	expose := domain.Expose{Desktop: []domain.ExposeEntry{{Name: "cc-switch", Path: domain.ExposeAuto}}}
	placed := filepath.Join(f.Apps[0], "CC Switch.app")

	first, err := f.shelf.Apply(context.Background(), mocks.NsA, wd, expose, domain.ArrowMedia{})
	require.NoError(t, err)
	reapplied, err := f.shelf.Apply(context.Background(), mocks.NsA, wd, expose, domain.ArrowMedia{})
	require.NoError(t, err)
	updated, err := f.shelf.Apply(context.Background(), mocks.NsA2, wd2, expose, domain.ArrowMedia{})
	require.NoError(t, err)

	for _, got := range []Applied{first, reapplied, updated} {
		assert.Empty(t, got.Refused)
		require.Len(t, got.Entries, 1)
		assert.Equal(t, "CC Switch", got.Entries[0].Name)
		assert.Equal(t, placed, got.Entries[0].Location)
	}
	assert.NoDirExists(t, filepath.Join(f.Apps[0], "cc-switch.app"))
	assert.Equal(t, "v2", mocks.ReadBundleVersion(t, placed))
}

func TestShelf_Apply_DeclaredDesktopBundleKeepsTheDeclaredName(t *testing.T) {
	f := newFixture(t, platform.GOOSDarwin)
	wd := f.Workdir(t, mocks.NsA)
	mocks.WriteBundle(t, filepath.Join(wd, "CC Switch.app"), "v1")
	expose := domain.Expose{Desktop: []domain.ExposeEntry{{Name: "cc-switch", Path: "${INSTALL_PATH}/CC Switch.app"}}}

	got, err := f.shelf.Apply(context.Background(), mocks.NsA, wd, expose, domain.ArrowMedia{})

	require.NoError(t, err)
	require.Len(t, got.Entries, 1)
	assert.Equal(t, filepath.Join(f.Apps[0], "cc-switch.app"), got.Entries[0].Location)
}

func TestOptions_SetFields(t *testing.T) {
	cmd := &mocks.Commander{}
	tagger := &mocks.Tagger{}
	up := &mocks.UserPath{}

	s := New(
		WithHomeDir("/q"),
		WithUserHomeDir("/u"),
		WithAppsDirs([]string{"/a", "/b"}),
		WithGOOS("plan9"),
		WithGOARCH("mips"),
		WithCommander(cmd),
		WithEnv(func(string) string { return "v" }),
		withTagger(tagger),
		withUserPath(up),
	).(*shelf)

	assert.Equal(t, "/q", s.host.HomeDir)
	assert.Equal(t, "/u", s.host.UserHomeDir)
	assert.Equal(t, []string{"/a", "/b"}, s.host.AppsDirs)
	assert.Equal(t, "plan9", s.host.GOOS)
	assert.Equal(t, "mips", s.host.GOARCH)
	assert.Same(t, cmd, s.host.Commander)
	assert.Equal(t, "v", s.host.Env("ANY"))
	assert.Same(t, tagger, s.tagger)
	assert.Same(t, up, s.userPath)
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
			s := New(
				WithGOOS(tc.goos),
				WithEnv(func(key string) string { return "/real/" + key }),
				WithSandboxHome(home),
			).(*shelf)

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

func TestWithSandboxHome_AppDataIgnoresOptionOrder(t *testing.T) {
	realEnv := WithEnv(func(key string) string { return "real-" + key })
	want := filepath.Join("/h", "AppData", "Roaming")
	testCases := []struct {
		name string
		opts []Option
	}{
		{name: "env before sandbox", opts: []Option{realEnv, WithSandboxHome("/h")}},
		{name: "env after sandbox", opts: []Option{WithSandboxHome("/h"), realEnv}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			s := New(tc.opts...).(*shelf)

			assert.Equal(t, want, s.host.AppData("/u"))
			assert.Equal(t, "real-PATH", s.host.Env("PATH"))
			assert.Equal(t, "real-SHELL", s.host.Env("SHELL"))
		})
	}
}

func TestRecord_Apply_LinuxDesktopEntryUsesRecordNameAndIcon(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newFixture(t, "linux")
	wd := f.Workdir(t, mocks.NsA)
	mocks.WriteFile(t, filepath.Join(wd, "Logseq.AppDir", ".quiver-run"), "x", 0o755)
	mocks.WriteFile(t, filepath.Join(wd, "Logseq.AppDir", "logseq.png"), "x", 0o644)
	mocks.WriteFile(t, filepath.Join(wd, "Other.AppImage"), "x", 0o755)
	mocks.WriteRecord(t, wd, domain.PortableApp{Name: "Logseq", Entry: "Logseq.AppDir/.quiver-run", Icon: "Logseq.AppDir/logseq.png"})
	expose := domain.Expose{Desktop: []domain.ExposeEntry{{Name: "logseq", Path: domain.ExposeAuto}}}

	got, err := f.shelf.Apply(context.Background(), mocks.NsA, wd, expose, domain.ArrowMedia{Icon: "/media.png"})

	require.NoError(t, err)
	require.Len(t, got.Entries, 1)
	loc := got.Entries[0].Location
	assert.Equal(t, xdgDir(f.UserHome), filepath.Dir(loc))
	data, err := os.ReadFile(loc)
	require.NoError(t, err)
	target := filepath.Join(wd, "Logseq.AppDir", ".quiver-run")
	icon := filepath.Join(wd, "Logseq.AppDir", "logseq.png")
	assert.Contains(t, string(data), "Name=Logseq\n")
	assert.Contains(t, string(data), "Exec=\""+target+"\"")
	assert.Contains(t, string(data), "Icon="+icon+"\n")
}

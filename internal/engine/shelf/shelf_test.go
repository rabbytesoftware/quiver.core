package shelf

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/paths"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

const (
	nsA        domain.Namespace = "github.com/acme/tool@v1"
	nsA2       domain.Namespace = "github.com/acme/tool@v2"
	nsB        domain.Namespace = "github.com/other/thing@v1"
	bareA      domain.Namespace = "github.com/acme/tool"
	bareB      domain.Namespace = "github.com/other/thing"
	stubTagKey                  = ".stub-owner"
)

type stubCommander struct {
	calls   [][]string
	envs    [][]string
	respond func(name string, args, env []string) ([]byte, error)
}

func (c *stubCommander) Run(
	ctx context.Context,
	name string,
	args ...string,
) ([]byte, error) {
	return c.RunWithEnv(ctx, nil, name, args...)
}

func (c *stubCommander) RunWithEnv(
	_ context.Context,
	env []string,
	name string,
	args ...string,
) ([]byte, error) {
	c.calls = append(c.calls, append([]string{name}, args...))
	c.envs = append(c.envs, env)
	if c.respond == nil {
		return nil, nil
	}
	return c.respond(name, args, env)
}

type stubTagger struct {
	readErr  error
	writeErr error
}

func (s *stubTagger) read(
	path string,
) (string, error) {
	if s.readErr != nil {
		return "", s.readErr
	}
	data, err := os.ReadFile(filepath.Join(path, stubTagKey))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	return string(data), err
}

func (s *stubTagger) write(
	path string,
	value string,
) error {
	if s.writeErr != nil {
		return s.writeErr
	}
	return os.WriteFile(filepath.Join(path, stubTagKey), []byte(value), 0o600)
}

type stubUserPath struct {
	value    string
	readErr  error
	writeErr error
	writes   int
}

func (u *stubUserPath) read() (string, error) {
	return u.value, u.readErr
}

func (u *stubUserPath) write(
	value string,
) error {
	if u.writeErr != nil {
		return u.writeErr
	}
	u.writes++
	u.value = value
	return nil
}

type fixture struct {
	home     string
	userHome string
	apps     []string
	nsDir    string
	bin      string
	cmd      *stubCommander
	tagger   *stubTagger
	userPath *stubUserPath
	env      map[string]string
	shelf    *shelf
}

func newFixture(
	t *testing.T,
	goos string,
	opts ...Option,
) *fixture {
	t.Helper()
	root := t.TempDir()
	f := &fixture{
		home:     filepath.Join(root, "quiver"),
		userHome: filepath.Join(root, "user"),
		apps: []string{
			filepath.Join(root, "Applications"),
			filepath.Join(root, "user", "Applications"),
		},
		cmd:      &stubCommander{},
		tagger:   &stubTagger{},
		userPath: &stubUserPath{},
		env:      map[string]string{},
	}

	nsDir, err := paths.NamespacesAt(f.home)
	require.NoError(t, err)
	f.nsDir = nsDir

	bin, err := paths.BinAt(f.home)
	require.NoError(t, err)
	f.bin = bin

	base := []Option{
		WithHomeDir(f.home),
		WithUserHomeDir(f.userHome),
		WithAppsDirs(f.apps),
		WithGOOS(goos),
		WithGOARCH("amd64"),
		WithCommander(f.cmd),
		WithEnv(f.lookup),
		withTagger(f.tagger),
		withUserPath(f.userPath),
	}
	f.shelf = New(append(base, opts...)...).(*shelf)
	return f
}

func (f *fixture) lookup(
	key string,
) string {
	return f.env[key]
}

func (f *fixture) workdir(
	t *testing.T,
	ns domain.Namespace,
) string {
	t.Helper()
	dir := filepath.Join(f.nsDir, filepath.FromSlash(string(ns)))
	require.NoError(t, os.MkdirAll(dir, 0o750))
	return dir
}

func writeFile(
	t *testing.T,
	path string,
	content string,
	mode os.FileMode,
) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	require.NoError(t, os.Chmod(path, mode))
}

func requireUnixHost(
	t *testing.T,
) {
	t.Helper()
	if runtime.GOOS == goosWindows {
		t.Skip("needs symlinks and executable bits of a unix host")
	}
}

func cliExpose(
	name string,
	path string,
) domain.Expose {
	return domain.Expose{CLI: []domain.ExposeEntry{{Name: name, Path: path}}}
}

func TestNew_Defaults(t *testing.T) {
	s := New().(*shelf)

	assert.Equal(t, runtime.GOOS, s.goos)
	assert.Equal(t, runtime.GOARCH, s.goarch)
	assert.Equal(t, defaultCommander, s.commander)
	assert.Equal(t, defaultTagger, s.tagger)
	assert.Equal(t, defaultUserPath, s.userPath)
	assert.NotNil(t, s.env)
	assert.Empty(t, s.homeDir)
	assert.Empty(t, s.userHomeDir)
	assert.Nil(t, s.appsDirs)
}

func TestShelf_Apply_EmptyExposeStillValidates(t *testing.T) {
	f := newFixture(t, "linux")

	_, err := f.shelf.Apply(context.Background(), nsA, "relative", domain.Expose{}, domain.ArrowMedia{})

	require.Error(t, err)
}

func TestShelf_Apply_PrunesStaleOwnedEntries(t *testing.T) {
	requireUnixHost(t)
	f := newFixture(t, "linux")
	wd1 := f.workdir(t, nsA)
	wd2 := f.workdir(t, nsA2)
	wdB := f.workdir(t, nsB)
	for _, wd := range []string{wd1, wd2} {
		writeFile(t, filepath.Join(wd, "keep"), "x", 0o755)
		writeFile(t, filepath.Join(wd, "old"), "x", 0o755)
		writeFile(t, filepath.Join(wd, "Tool.AppImage"), "x", 0o755)
	}
	writeFile(t, filepath.Join(wdB, "thing"), "x", 0o755)
	writeFile(t, filepath.Join(f.bin, "mine"), "user", 0o755)
	v1 := domain.Expose{
		CLI:     []domain.ExposeEntry{{Name: "keep", Path: "keep"}, {Name: "old", Path: "old"}},
		Desktop: []domain.ExposeEntry{{Name: "Tool", Path: "Tool.AppImage"}},
	}
	v2 := domain.Expose{CLI: []domain.ExposeEntry{{Name: "keep", Path: "keep"}}}

	first, err := f.shelf.Apply(context.Background(), nsA, wd1, v1, domain.ArrowMedia{})
	require.NoError(t, err)
	require.Len(t, first.Entries, 3)
	_, err = f.shelf.Apply(context.Background(), nsB, wdB, cliExpose("thing", "thing"), domain.ArrowMedia{})
	require.NoError(t, err)
	second, err := f.shelf.Apply(context.Background(), nsA2, wd2, v2, domain.ArrowMedia{})
	require.NoError(t, err)

	require.Len(t, second.Entries, 1)
	link, err := os.Readlink(filepath.Join(f.bin, "keep"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(wd2, "keep"), link)
	assert.NoFileExists(t, filepath.Join(f.bin, "old"))
	assert.NoFileExists(t, first.Entries[0].Location)
	assert.FileExists(t, filepath.Join(f.bin, "thing"))
	assert.FileExists(t, filepath.Join(f.bin, "mine"))

	_, err = f.shelf.Apply(context.Background(), nsA2, wd2, domain.Expose{}, domain.ArrowMedia{})
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(f.bin, "keep"))
	assert.FileExists(t, filepath.Join(f.bin, "thing"))
}

func TestShelf_Apply_PruneError_ReturnsError(t *testing.T) {
	f := newFixture(t, goosWindows)
	wd := f.workdir(t, nsA)
	writeFile(t, filepath.Join(lnkDir(filepath.Join(f.userHome, "AppData", "Roaming")), "stale.lnk"), "", 0o600)
	f.cmd.respond = func(string, []string, []string) ([]byte, error) { return nil, errors.New("boom") }

	_, err := f.shelf.Apply(context.Background(), nsA, wd, domain.Expose{}, domain.ArrowMedia{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "prune")
}

func TestShelf_Apply_SymlinkEscapesRefused(t *testing.T) {
	requireUnixHost(t)
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
			goos:    goosDarwin,
			outside: "User.app/Contents/version",
			link:    "lib",
			expose:  domain.Expose{Desktop: []domain.ExposeEntry{{Name: "User", Path: "${INSTALL_PATH}/lib/User.app"}}},
			check: func(t *testing.T, f *fixture, outside string) {
				assert.DirExists(t, filepath.Join(outside, "User.app"))
				assert.NoFileExists(t, filepath.Join(outside, "User.app", stubTagKey))
				assert.NoDirExists(t, filepath.Join(f.apps[0], "User.app"))
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
			goos:    goosDarwin,
			outside: "tool",
			link:    "bin",
			expose:  cliExpose("tool", "${INSTALL_PATH}/bin/tool"),
			check: func(t *testing.T, f *fixture, _ string) {
				assert.Empty(t, f.cmd.calls)
				assert.NoFileExists(t, filepath.Join(f.bin, "tool"))
			},
		},
		{
			name:    "final symlink leaving the workdir",
			goos:    "linux",
			outside: "tool",
			link:    "tool",
			expose:  cliExpose("tool", "${INSTALL_PATH}/tool"),
			check: func(t *testing.T, f *fixture, _ string) {
				assert.NoFileExists(t, filepath.Join(f.bin, "tool"))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, tc.goos, WithGOARCH(goarchARM64))
			wd := f.workdir(t, nsA)
			outside := t.TempDir()
			writeFile(t, filepath.Join(outside, filepath.FromSlash(tc.outside)), machO, 0o644)
			linkTarget := outside
			if tc.link == "tool" {
				linkTarget = filepath.Join(outside, "tool")
			}
			require.NoError(t, os.Symlink(linkTarget, filepath.Join(wd, tc.link)))

			got, err := f.shelf.Apply(context.Background(), nsA, wd, tc.expose, domain.ArrowMedia{})

			require.NoError(t, err)
			assert.Empty(t, got.Entries)
			require.Len(t, got.Refused, 1)
			assert.Equal(t, reasonOutsideWorkdir, got.Refused[0].Reason)
			tc.check(t, f, outside)
		})
	}
}

func TestShelf_Apply_SymlinkedBundleInsideWorkdirRefused(t *testing.T) {
	requireUnixHost(t)
	f := newFixture(t, goosDarwin)
	wd := f.workdir(t, nsA)
	writeBundle(t, filepath.Join(wd, "real", "Tool.app"), "v1")
	require.NoError(t, os.Symlink(filepath.Join(wd, "real", "Tool.app"), filepath.Join(wd, "Tool.app")))

	got, err := f.shelf.Apply(context.Background(), nsA, wd, domain.Expose{Desktop: []domain.ExposeEntry{{Name: "Tool", Path: "Tool.app"}}}, domain.ArrowMedia{})

	require.NoError(t, err)
	assert.Equal(t, []Refusal{{Kind: domain.ExposeKindDesktop, Name: "Tool", Reason: reasonWrongType}}, got.Refused)
	assert.DirExists(t, filepath.Join(wd, "real", "Tool.app"))
}

func TestShelf_Apply_InvalidRequest_ReturnsError(t *testing.T) {
	f := newFixture(t, "linux")
	wdB := f.workdir(t, nsB)

	testCases := []struct {
		name    string
		ns      domain.Namespace
		workdir string
		want    string
	}{
		{
			name:    "invalid namespace",
			ns:      "nope",
			workdir: wdB,
			want:    "shelf: apply nope",
		},
		{
			name:    "workdir of another namespace",
			ns:      nsA,
			workdir: wdB,
			want:    "is not a workdir of",
		},
		{
			name:    "workdir outside namespaces",
			ns:      nsA,
			workdir: t.TempDir(),
			want:    "is not a workdir of",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.shelf.Apply(context.Background(), tc.ns, tc.workdir, cliExpose("tool", "tool"), domain.ArrowMedia{})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestShelf_Apply_LayoutError_ReturnsError(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	s := New(WithHomeDir(file))

	_, err := s.Apply(context.Background(), nsA, file, cliExpose("tool", "tool"), domain.ArrowMedia{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "shelf: apply")
}

func TestShelf_Apply_CLISymlinkCreated(t *testing.T) {
	requireUnixHost(t)
	f := newFixture(t, "linux")
	wd := f.workdir(t, nsA)
	target := filepath.Join(wd, "bin", "tool")
	writeFile(t, target, "#!/bin/sh\n", 0o755)

	got, err := f.shelf.Apply(context.Background(), nsA, wd, cliExpose("tool", "${INSTALL_PATH}/bin/tool"), domain.ArrowMedia{})

	require.NoError(t, err)
	assert.Empty(t, got.Refused)
	assert.Equal(t, []AppliedEntry{{
		Kind:     domain.ExposeKindCLI,
		Name:     "tool",
		Target:   target,
		Location: filepath.Join(f.bin, "tool"),
	}}, got.Entries)
	link, err := os.Readlink(filepath.Join(f.bin, "tool"))
	require.NoError(t, err)
	assert.Equal(t, target, link)
}

func TestShelf_Apply_DeclaredCLIMadeExecutable(t *testing.T) {
	requireUnixHost(t)
	f := newFixture(t, "linux")
	wd := f.workdir(t, nsA)
	target := filepath.Join(wd, "tool")
	writeFile(t, target, "#!/bin/sh\n", 0o644)

	got, err := f.shelf.Apply(context.Background(), nsA, wd, cliExpose("tool", "${INSTALL_PATH}/tool"), domain.ArrowMedia{})

	require.NoError(t, err)
	assert.Empty(t, got.Refused)
	info, err := os.Stat(target)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
	link, err := os.Readlink(filepath.Join(f.bin, "tool"))
	require.NoError(t, err)
	assert.Equal(t, target, link)
}

func TestShelf_Apply_DeclaredCLIOutsideWorkdirNotMarked(t *testing.T) {
	requireUnixHost(t)
	f := newFixture(t, "linux")
	wd := f.workdir(t, nsA)
	outside := filepath.Join(t.TempDir(), "tool")
	writeFile(t, outside, "#!/bin/sh\n", 0o644)
	require.NoError(t, os.Symlink(outside, filepath.Join(wd, "tool")))

	got, err := f.shelf.Apply(context.Background(), nsA, wd, cliExpose("tool", "${INSTALL_PATH}/tool"), domain.ArrowMedia{})

	require.NoError(t, err)
	require.Len(t, got.Refused, 1)
	assert.Equal(t, reasonOutsideWorkdir, got.Refused[0].Reason)
	info, err := os.Stat(outside)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())
}

func TestShelf_Apply_AutoCLIStillNeedsExecBit(t *testing.T) {
	requireUnixHost(t)
	f := newFixture(t, "linux")
	wd := f.workdir(t, nsA)
	target := filepath.Join(wd, "tool")
	writeFile(t, target, "#!/bin/sh\n", 0o644)

	got, err := f.shelf.Apply(context.Background(), nsA, wd, cliExpose("tool", domain.ExposeAuto), domain.ArrowMedia{})

	require.NoError(t, err)
	assert.Empty(t, got.Refused)
	assert.Empty(t, got.Entries)
	info, err := os.Stat(target)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())
}

func TestShelf_Apply_ReapplyNewWorkdirRepoints(t *testing.T) {
	requireUnixHost(t)
	f := newFixture(t, "linux")
	wd1 := f.workdir(t, nsA)
	wd2 := f.workdir(t, nsA2)
	writeFile(t, filepath.Join(wd1, "tool"), "v1", 0o755)
	writeFile(t, filepath.Join(wd2, "tool"), "v2", 0o755)

	_, err := f.shelf.Apply(context.Background(), nsA, wd1, cliExpose("tool", "${WORKDIR}/tool"), domain.ArrowMedia{})
	require.NoError(t, err)
	got, err := f.shelf.Apply(context.Background(), nsA2, wd2, cliExpose("tool", "${WORKDIR}/tool"), domain.ArrowMedia{})
	require.NoError(t, err)

	assert.Empty(t, got.Refused)
	require.Len(t, got.Entries, 1)
	link, err := os.Readlink(filepath.Join(f.bin, "tool"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(wd2, "tool"), link)
}

func TestShelf_Apply_IsIdempotent(t *testing.T) {
	requireUnixHost(t)
	f := newFixture(t, "linux")
	wd := f.workdir(t, nsA)
	writeFile(t, filepath.Join(wd, "tool"), "x", 0o755)
	expose := cliExpose("tool", "tool")

	first, err := f.shelf.Apply(context.Background(), nsA, wd, expose, domain.ArrowMedia{})
	require.NoError(t, err)
	second, err := f.shelf.Apply(context.Background(), nsA, wd, expose, domain.ArrowMedia{})
	require.NoError(t, err)

	assert.Equal(t, first, second)
	assert.Empty(t, second.Refused)
}

func TestShelf_Apply_ForeignOwnerRefused(t *testing.T) {
	requireUnixHost(t)
	f := newFixture(t, "linux")
	wdA := f.workdir(t, nsA)
	wdB := f.workdir(t, nsB)
	writeFile(t, filepath.Join(wdA, "tool"), "a", 0o755)
	writeFile(t, filepath.Join(wdB, "tool"), "b", 0o755)

	_, err := f.shelf.Apply(context.Background(), nsB, wdB, cliExpose("tool", "tool"), domain.ArrowMedia{})
	require.NoError(t, err)
	got, err := f.shelf.Apply(context.Background(), nsA, wdA, cliExpose("tool", "tool"), domain.ArrowMedia{})
	require.NoError(t, err)

	assert.Empty(t, got.Entries)
	assert.Equal(t, []Refusal{{
		Kind:   domain.ExposeKindCLI,
		Name:   "tool",
		Reason: "owned by github.com/other/thing",
	}}, got.Refused)
	link, err := os.Readlink(filepath.Join(f.bin, "tool"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(wdB, "tool"), link)
}

func TestShelf_Apply_UnownedFileRefused(t *testing.T) {
	requireUnixHost(t)
	f := newFixture(t, "linux")
	wd := f.workdir(t, nsA)
	writeFile(t, filepath.Join(wd, "tool"), "a", 0o755)
	writeFile(t, filepath.Join(f.bin, "tool"), "mine", 0o755)

	got, err := f.shelf.Apply(context.Background(), nsA, wd, cliExpose("tool", "tool"), domain.ArrowMedia{})
	require.NoError(t, err)

	assert.Empty(t, got.Entries)
	assert.Equal(t, []Refusal{{Kind: domain.ExposeKindCLI, Name: "tool", Reason: reasonUnmanaged}}, got.Refused)
	data, err := os.ReadFile(filepath.Join(f.bin, "tool"))
	require.NoError(t, err)
	assert.Equal(t, "mine", string(data))
}

func TestShelf_Apply_ResolveRefusalRecorded(t *testing.T) {
	f := newFixture(t, "linux")
	wd := f.workdir(t, nsA)

	got, err := f.shelf.Apply(context.Background(), nsA, wd, cliExpose("../x", "tool"), domain.ArrowMedia{})
	require.NoError(t, err)

	assert.Equal(t, []Refusal{{Kind: domain.ExposeKindCLI, Name: "../x", Reason: reasonUnsafeName}}, got.Refused)
}

func TestShelf_Apply_ResolveError_ReturnsError(t *testing.T) {
	f := newFixture(t, "linux")
	wd := f.workdir(t, nsA)
	require.NoError(t, os.RemoveAll(wd))

	_, err := f.shelf.Apply(context.Background(), nsA, wd, cliExpose("tool", domain.ExposeAuto), domain.ArrowMedia{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "shelf: apply cli tool")
}

func TestShelf_Apply_PlaceError_ReturnsError(t *testing.T) {
	testCases := []struct {
		name   string
		expose func(wd string) domain.Expose
	}{
		{
			name: "cli",
			expose: func(string) domain.Expose {
				return cliExpose("tool", "tool")
			},
		},
		{
			name: "desktop",
			expose: func(string) domain.Expose {
				return domain.Expose{Desktop: []domain.ExposeEntry{{Name: "tool", Path: "tool"}}}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, goosWindows)
			wd := f.workdir(t, nsA)
			writeFile(t, filepath.Join(wd, "tool"), "x", 0o755)
			writeFile(t, filepath.Join(f.bin, "tool.cmd"+stagedSuffix, "x"), "", 0o600)
			f.cmd.respond = func(string, []string, []string) ([]byte, error) { return nil, errors.New("boom") }
			writeFile(t, filepath.Join(lnkDir(filepath.Join(f.userHome, "AppData", "Roaming")), "tool.lnk"), "", 0o600)

			_, err := f.shelf.Apply(context.Background(), nsA, wd, tc.expose(wd), domain.ArrowMedia{})

			require.Error(t, err)
			assert.Contains(t, err.Error(), "shelf: apply "+tc.name+" tool")
		})
	}
}

func TestShelf_Place_UnknownKind_ReturnsError(t *testing.T) {
	f := newFixture(t, "linux")

	_, err := f.shelf.place(context.Background(), applyRequest{}, "bogus", domain.ExposeEntry{}, candidate{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown expose kind")
}

func TestShelf_Apply_DesktopBundleRelocatesCLITargets(t *testing.T) {
	requireUnixHost(t)
	f := newFixture(t, goosDarwin)
	wd := f.workdir(t, nsA)
	writeFile(t, filepath.Join(wd, "Tool.app", "Contents", "MacOS", "tool"), "x", 0o755)
	expose := domain.Expose{
		CLI:     []domain.ExposeEntry{{Name: "tool", Path: "${INSTALL_PATH}/Tool.app/Contents/MacOS/tool"}},
		Desktop: []domain.ExposeEntry{{Name: "Tool", Path: "${INSTALL_PATH}/Tool.app"}},
	}
	bundle := filepath.Join(f.apps[0], "Tool.app")
	inBundle := filepath.Join(bundle, "Contents", "MacOS", "tool")

	first, err := f.shelf.Apply(context.Background(), nsA, wd, expose, domain.ArrowMedia{})
	require.NoError(t, err)
	second, err := f.shelf.Apply(context.Background(), nsA, wd, expose, domain.ArrowMedia{})
	require.NoError(t, err)

	assert.Empty(t, first.Refused)
	assert.Empty(t, second.Refused)
	assert.Equal(t, first, second)
	require.Len(t, first.Entries, 2)
	assert.Equal(t, domain.ExposeKindDesktop, first.Entries[0].Kind)
	assert.Equal(t, bundle, first.Entries[0].Location)
	assert.Equal(t, inBundle, first.Entries[1].Target)
	link, err := os.Readlink(filepath.Join(f.bin, "tool"))
	require.NoError(t, err)
	assert.Equal(t, inBundle, link)

	require.NoError(t, f.shelf.Remove(context.Background(), nsA))
	assert.NoFileExists(t, filepath.Join(f.bin, "tool"))
	assert.NoDirExists(t, bundle)
}

func TestShelf_Remove_CleansOnlyOwnEntries(t *testing.T) {
	requireUnixHost(t)
	f := newFixture(t, "linux")
	wdA := f.workdir(t, nsA)
	wdB := f.workdir(t, nsB)
	writeFile(t, filepath.Join(wdA, "tool"), "a", 0o755)
	writeFile(t, filepath.Join(wdA, "Tool.AppImage"), "a", 0o755)
	writeFile(t, filepath.Join(wdB, "thing"), "b", 0o755)
	writeFile(t, filepath.Join(f.bin, "mine"), "user", 0o755)
	userDesktop := filepath.Join(xdgDir(f.userHome), "user.desktop")
	writeFile(t, userDesktop, "[Desktop Entry]\n", 0o600)

	exposeA := domain.Expose{
		CLI:     []domain.ExposeEntry{{Name: "tool", Path: "tool"}},
		Desktop: []domain.ExposeEntry{{Name: "Tool", Path: "Tool.AppImage"}},
	}
	gotA, err := f.shelf.Apply(context.Background(), nsA, wdA, exposeA, domain.ArrowMedia{})
	require.NoError(t, err)
	require.Len(t, gotA.Entries, 2)
	_, err = f.shelf.Apply(context.Background(), nsB, wdB, cliExpose("thing", "thing"), domain.ArrowMedia{})
	require.NoError(t, err)

	require.NoError(t, f.shelf.Remove(context.Background(), nsA2))

	for _, e := range gotA.Entries {
		assert.NoFileExists(t, e.Location)
	}
	assert.FileExists(t, filepath.Join(f.bin, "thing"))
	assert.FileExists(t, filepath.Join(f.bin, "mine"))
	assert.FileExists(t, userDesktop)
}

func TestShelf_Remove_InvalidRequest_ReturnsError(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	testCases := []struct {
		name  string
		shelf Shelf
		ns    domain.Namespace
	}{
		{
			name:  "invalid namespace",
			shelf: newFixture(t, "linux").shelf,
			ns:    "nope",
		},
		{
			name:  "layout error",
			shelf: New(WithHomeDir(file)),
			ns:    nsA,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.shelf.Remove(context.Background(), tc.ns)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "shelf: remove")
		})
	}
}

func TestShelf_Remove_JoinsErrors(t *testing.T) {
	f := newFixture(t, goosWindows)
	writeFile(t, filepath.Join(lnkDir(filepath.Join(f.userHome, "AppData", "Roaming")), "tool.lnk"), "", 0o600)
	f.cmd.respond = func(string, []string, []string) ([]byte, error) { return nil, errors.New("boom") }

	err := f.shelf.Remove(context.Background(), nsA)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

func TestShelf_PathMethods_Delegate(t *testing.T) {
	f := newFixture(t, "linux")
	f.env["SHELL"] = "/bin/zsh"

	status, err := f.shelf.PathStatus(context.Background())
	require.NoError(t, err)
	assert.False(t, status.Configured)

	status, err = f.shelf.SetupPath(context.Background())
	require.NoError(t, err)
	assert.True(t, status.Configured)
	assert.Equal(t, f.bin, status.BinDir)
}

func TestShelf_DefaultHome_UsesProcessHome(t *testing.T) {
	quiverHome := t.TempDir()
	userHome := t.TempDir()
	t.Setenv("QUIVER_HOME", quiverHome)
	t.Setenv("HOME", userHome)
	t.Setenv("USERPROFILE", userHome)

	s := New(WithGOOS("linux"), WithEnv(func(string) string { return "" })).(*shelf)
	l, err := s.layout()
	require.NoError(t, err)

	wantBin, err := paths.BinAt(quiverHome)
	require.NoError(t, err)
	wantNamespaces, err := paths.NamespacesAt(quiverHome)
	require.NoError(t, err)
	assert.Equal(t, wantBin, l.bin)
	assert.Equal(t, wantNamespaces, l.namespaces)
	assert.Equal(t, userHome, l.userHome)
	assert.Equal(t, []string{"/Applications", filepath.Join(userHome, "Applications")}, l.apps)
}

func TestShelf_Apply_AutoCLINamedAfterTheExecutable(t *testing.T) {
	requireUnixHost(t)
	f := newFixture(t, "linux")
	wd := f.workdir(t, nsA)
	wd2 := f.workdir(t, nsA2)
	writeFile(t, filepath.Join(wd, "ripgrep-14", "rg"), "x", 0o755)
	writeFile(t, filepath.Join(wd2, "fd"), "x", 0o755)

	first, err := f.shelf.Apply(context.Background(), nsA, wd, cliExpose("ripgrep", domain.ExposeAuto), domain.ArrowMedia{})
	require.NoError(t, err)

	require.Len(t, first.Entries, 1)
	assert.Empty(t, first.Refused)
	assert.Equal(t, "rg", first.Entries[0].Name)
	assert.Equal(t, filepath.Join(f.bin, "rg"), first.Entries[0].Location)
	link, err := os.Readlink(filepath.Join(f.bin, "rg"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(wd, "ripgrep-14", "rg"), link)
	assert.NoFileExists(t, filepath.Join(f.bin, "ripgrep"))

	second, err := f.shelf.Apply(context.Background(), nsA2, wd2, cliExpose("ripgrep", domain.ExposeAuto), domain.ArrowMedia{})
	require.NoError(t, err)

	require.Len(t, second.Entries, 1)
	assert.Equal(t, "fd", second.Entries[0].Name)
	assert.FileExists(t, filepath.Join(f.bin, "fd"))
	_, err = os.Lstat(filepath.Join(f.bin, "rg"))
	assert.ErrorIs(t, err, fs.ErrNotExist)
}

func TestShelf_Apply_AutoCLIForeignOwnerRefusedUnderTheExecutableName(t *testing.T) {
	requireUnixHost(t)
	f := newFixture(t, "linux")
	wdA := f.workdir(t, nsA)
	wdB := f.workdir(t, nsB)
	writeFile(t, filepath.Join(wdA, "rg"), "a", 0o755)
	writeFile(t, filepath.Join(wdB, "rg"), "b", 0o755)

	_, err := f.shelf.Apply(context.Background(), nsB, wdB, cliExpose("rg", "rg"), domain.ArrowMedia{})
	require.NoError(t, err)
	got, err := f.shelf.Apply(context.Background(), nsA, wdA, cliExpose("ripgrep", domain.ExposeAuto), domain.ArrowMedia{})
	require.NoError(t, err)

	assert.Empty(t, got.Entries)
	assert.Equal(t, []Refusal{{Kind: domain.ExposeKindCLI, Name: "rg", Reason: "owned by github.com/other/thing"}}, got.Refused)
}

func TestShelf_Apply_AutoCLIOnWindowsStripsExe(t *testing.T) {
	f := newFixture(t, goosWindows)
	wd := f.workdir(t, nsA)
	writeFile(t, filepath.Join(wd, "rg.exe"), "x", 0o755)

	got, err := f.shelf.Apply(context.Background(), nsA, wd, cliExpose("ripgrep", domain.ExposeAuto), domain.ArrowMedia{})
	require.NoError(t, err)

	require.Len(t, got.Entries, 1)
	assert.Equal(t, "rg", got.Entries[0].Name)
	assert.Equal(t, filepath.Join(f.bin, "rg.cmd"), got.Entries[0].Location)
	assert.NoFileExists(t, filepath.Join(f.bin, "ripgrep.cmd"))
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
			goos:   goosDarwin,
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
			goos:        goosDarwin,
			expose:      domain.Expose{Desktop: []domain.ExposeEntry{{Name: "Tool", Path: "${INSTALL_PATH}/Tool.app"}}},
			wantRefused: []Refusal{{Kind: domain.ExposeKindDesktop, Name: "Tool", Reason: reasonNotFound}},
		},
		{
			name:        "declared cli that does not exist",
			goos:        "linux",
			expose:      cliExpose("tool", "${INSTALL_PATH}/tool"),
			wantRefused: []Refusal{{Kind: domain.ExposeKindCLI, Name: "tool", Reason: reasonNotFound}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, tc.goos)
			wd := f.workdir(t, nsA)
			writeFile(t, filepath.Join(wd, "README"), "x", 0o644)

			got, err := f.shelf.Apply(context.Background(), nsA, wd, tc.expose, domain.ArrowMedia{})

			require.NoError(t, err)
			assert.Empty(t, got.Entries)
			assert.Equal(t, tc.wantRefused, got.Refused)
		})
	}
}

func TestShelf_Apply_AutoDesktopBundleKeepsItsOwnName(t *testing.T) {
	f := newFixture(t, goosDarwin)
	wd := f.workdir(t, nsA)
	wd2 := f.workdir(t, nsA2)
	writeBundle(t, filepath.Join(wd, "CC Switch.app"), "v1")
	writeBundle(t, filepath.Join(wd2, "CC Switch.app"), "v2")
	expose := domain.Expose{Desktop: []domain.ExposeEntry{{Name: "cc-switch", Path: domain.ExposeAuto}}}
	placed := filepath.Join(f.apps[0], "CC Switch.app")

	first, err := f.shelf.Apply(context.Background(), nsA, wd, expose, domain.ArrowMedia{})
	require.NoError(t, err)
	reapplied, err := f.shelf.Apply(context.Background(), nsA, wd, expose, domain.ArrowMedia{})
	require.NoError(t, err)
	updated, err := f.shelf.Apply(context.Background(), nsA2, wd2, expose, domain.ArrowMedia{})
	require.NoError(t, err)

	for _, got := range []Applied{first, reapplied, updated} {
		assert.Empty(t, got.Refused)
		require.Len(t, got.Entries, 1)
		assert.Equal(t, "CC Switch", got.Entries[0].Name)
		assert.Equal(t, placed, got.Entries[0].Location)
	}
	assert.NoDirExists(t, filepath.Join(f.apps[0], "cc-switch.app"))
	assert.Equal(t, "v2", readBundleVersion(t, placed))
}

func TestShelf_Apply_DeclaredDesktopBundleKeepsTheDeclaredName(t *testing.T) {
	f := newFixture(t, goosDarwin)
	wd := f.workdir(t, nsA)
	writeBundle(t, filepath.Join(wd, "CC Switch.app"), "v1")
	expose := domain.Expose{Desktop: []domain.ExposeEntry{{Name: "cc-switch", Path: "${INSTALL_PATH}/CC Switch.app"}}}

	got, err := f.shelf.Apply(context.Background(), nsA, wd, expose, domain.ArrowMedia{})

	require.NoError(t, err)
	require.Len(t, got.Entries, 1)
	assert.Equal(t, filepath.Join(f.apps[0], "cc-switch.app"), got.Entries[0].Location)
}

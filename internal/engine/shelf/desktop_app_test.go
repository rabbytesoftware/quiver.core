package shelf

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func appRequest(
	t *testing.T,
	f *fixture,
	ns domain.Namespace,
) (applyRequest, string) {
	t.Helper()
	wd := f.workdir(t, ns)
	l, err := f.shelf.layout()
	require.NoError(t, err)
	return applyRequest{layout: l, bare: ns.BareNamespace(), workdir: wd, moved: map[string]string{}}, wd
}

func writeBundle(
	t *testing.T,
	path string,
	version string,
) {
	t.Helper()
	writeFile(t, filepath.Join(path, "Contents", "version"), version, 0o600)
}

func readBundleVersion(
	t *testing.T,
	path string,
) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(path, "Contents", "version"))
	require.NoError(t, err)
	return string(data)
}

func TestBundleName(t *testing.T) {
	assert.Equal(t, "Tool.app", bundleName("Tool"))
	assert.Equal(t, "Tool.app", bundleName("Tool.app"))
}

func TestShelf_PlaceApp_MovesAndTags(t *testing.T) {
	f := newFixture(t, goosDarwin)
	req, wd := appRequest(t, f, nsA)
	src := filepath.Join(wd, "Tool.app")
	writeBundle(t, src, "v1")
	dest := filepath.Join(f.apps[0], "Tool.app")

	got, err := f.shelf.placeApp(context.Background(), req, candidate{name: "Tool", target: src})

	require.NoError(t, err)
	assert.Equal(t, placement{location: dest}, got)
	assert.NoDirExists(t, src)
	assert.Equal(t, "v1", readBundleVersion(t, dest))
	tag, err := f.tagger.read(dest)
	require.NoError(t, err)
	assert.Equal(t, "github.com/acme/tool\nTool.app", tag)
	assert.Equal(t, map[string]string{src: dest}, req.moved)
	assert.NoDirExists(t, dest+stagedSuffix)
}

func TestShelf_PlaceApp_ReplacesOwnedBundle(t *testing.T) {
	f := newFixture(t, goosDarwin)
	req1, wd1 := appRequest(t, f, nsA)
	req2, wd2 := appRequest(t, f, nsA2)
	writeBundle(t, filepath.Join(wd1, "Tool.app"), "v1")
	writeBundle(t, filepath.Join(wd2, "Tool.app"), "v2")

	_, err := f.shelf.placeApp(context.Background(), req1, candidate{name: "Tool", target: filepath.Join(wd1, "Tool.app")})
	require.NoError(t, err)
	got, err := f.shelf.placeApp(context.Background(), req2, candidate{name: "Tool", target: filepath.Join(wd2, "Tool.app")})
	require.NoError(t, err)

	dest := filepath.Join(f.apps[0], "Tool.app")
	assert.Equal(t, placement{location: dest}, got)
	assert.Equal(t, "v2", readBundleVersion(t, dest))
}

func TestShelf_PlaceApp_ReplacesOwnedBundleWhereItLives(t *testing.T) {
	f := newFixture(t, goosDarwin)
	req, wd := appRequest(t, f, nsA)
	writeBundle(t, filepath.Join(wd, "Tool.app"), "v2")
	existing := filepath.Join(f.apps[1], "Tool.app")
	writeBundle(t, existing, "v1")
	require.NoError(t, f.tagger.write(existing, bundleTag(bareA, existing)))

	got, err := f.shelf.placeApp(context.Background(), req, candidate{name: "Tool", target: filepath.Join(wd, "Tool.app")})

	require.NoError(t, err)
	assert.Equal(t, placement{location: existing}, got)
	assert.Equal(t, "v2", readBundleVersion(t, existing))
	assert.NoDirExists(t, filepath.Join(f.apps[0], "Tool.app"))
}

func TestShelf_PlaceApp_Refusals(t *testing.T) {
	testCases := []struct {
		name     string
		owner    domain.Namespace
		existing bool
		source   string
		want     string
	}{
		{name: "foreign bundle", existing: true, owner: bareB, source: "dir", want: "owned by github.com/other/thing"},
		{name: "user bundle", existing: true, source: "dir", want: reasonUnmanaged},
		{name: "missing source", source: "none", want: reasonNotFound},
		{name: "file source", source: "file", want: reasonWrongType},
		{name: "unreadable owner tag counts as unmanaged", existing: true, owner: "unreadable", source: "dir", want: reasonUnmanaged},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, goosDarwin)
			req, wd := appRequest(t, f, nsA)
			src := filepath.Join(wd, "Tool.app")
			dest := filepath.Join(f.apps[0], "Tool.app")
			if tc.source == "dir" {
				writeBundle(t, src, "new")
			}
			if tc.source == "file" {
				writeFile(t, src, "x", 0o600)
			}
			if tc.existing {
				writeBundle(t, dest, "user")
			}
			if tc.owner != "" && tc.owner != "unreadable" {
				require.NoError(t, f.tagger.write(dest, bundleTag(tc.owner, dest)))
			}
			if tc.owner == "unreadable" {
				f.tagger.readErr = errors.New("boom")
			}

			got, err := f.shelf.placeApp(context.Background(), req, candidate{name: "Tool", target: src})

			require.NoError(t, err)
			assert.Equal(t, placement{refused: tc.want}, got)
			if tc.existing {
				assert.Equal(t, "user", readBundleVersion(t, dest))
			}
		})
	}
}

func TestShelf_PlaceApp_AlreadyMovedIsIdempotent(t *testing.T) {
	f := newFixture(t, goosDarwin)
	req, wd := appRequest(t, f, nsA)
	src := filepath.Join(wd, "Tool.app")
	writeBundle(t, src, "v1")

	first, err := f.shelf.placeApp(context.Background(), req, candidate{name: "Tool", target: src})
	require.NoError(t, err)
	req.moved = map[string]string{}
	second, err := f.shelf.placeApp(context.Background(), req, candidate{name: "Tool", target: src})
	require.NoError(t, err)

	assert.Equal(t, first, second)
	assert.Equal(t, map[string]string{src: first.location}, req.moved)
}

func readOnlyDir(
	t *testing.T,
) string {
	t.Helper()
	requireUnixHost(t)
	if os.Geteuid() == 0 {
		t.Skip("root writes into read-only directories")
	}
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o750) })
	return dir
}

func TestShelf_PlaceApp_FallsBackToNextWritableDir(t *testing.T) {
	f := newFixture(t, goosDarwin)
	f.shelf.appsDirs = []string{readOnlyDir(t), f.apps[1]}
	req, wd := appRequest(t, f, nsA)
	writeBundle(t, filepath.Join(wd, "Tool.app"), "v1")

	got, err := f.shelf.placeApp(context.Background(), req, candidate{name: "Tool", target: filepath.Join(wd, "Tool.app")})

	require.NoError(t, err)
	assert.Equal(t, placement{location: filepath.Join(f.apps[1], "Tool.app")}, got)
}

func TestShelf_PlaceApp_NoWritableDir(t *testing.T) {
	f := newFixture(t, goosDarwin)
	f.shelf.appsDirs = []string{readOnlyDir(t)}
	req, wd := appRequest(t, f, nsA)
	src := filepath.Join(wd, "Tool.app")
	writeBundle(t, src, "v1")

	_, err := f.shelf.placeApp(context.Background(), req, candidate{name: "Tool", target: src})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no writable applications directory")
	assert.DirExists(t, src)
}

func TestShelf_PlaceApp_TaggerErrors(t *testing.T) {
	boom := errors.New("boom")

	testCases := []struct {
		name     string
		existing bool
		readErr  error
		writeErr error
	}{
		{name: "tag write fails", writeErr: boom},
		{name: "tag write fails with an owned bundle in place", existing: true, writeErr: boom},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, goosDarwin)
			req, wd := appRequest(t, f, nsA)
			src := filepath.Join(wd, "Tool.app")
			writeBundle(t, src, "v1")
			if tc.existing {
				writeBundle(t, filepath.Join(f.apps[0], "Tool.app"), "x")
				require.NoError(t, f.tagger.write(filepath.Join(f.apps[0], "Tool.app"), bundleTag(bareA, filepath.Join(f.apps[0], "Tool.app"))))
			}
			f.tagger.readErr = tc.readErr
			f.tagger.writeErr = tc.writeErr

			_, err := f.shelf.placeApp(context.Background(), req, candidate{name: "Tool", target: src})

			require.ErrorIs(t, err, boom)
			assert.Equal(t, "v1", readBundleVersion(t, src))
		})
	}
}

func TestShelf_PlaceApp_LstatError(t *testing.T) {
	requireUnixHost(t)
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	f := newFixture(t, goosDarwin)
	f.shelf.appsDirs = []string{file}
	req, wd := appRequest(t, f, nsA)
	writeBundle(t, filepath.Join(wd, "Tool.app"), "v1")

	_, err := f.shelf.placeApp(context.Background(), req, candidate{name: "Tool", target: filepath.Join(wd, "Tool.app")})

	require.Error(t, err)
}

func TestShelf_BundleHolder_NonDirectoryIsUnmanaged(t *testing.T) {
	f := newFixture(t, goosDarwin)
	file := filepath.Join(t.TempDir(), "Tool.app")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	got, err := f.shelf.bundleHolder(file)

	require.NoError(t, err)
	assert.Equal(t, holder{exists: true}, got)
}

func TestShelf_RemoveApps(t *testing.T) {
	f := newFixture(t, goosDarwin)
	own0 := filepath.Join(f.apps[0], "Tool.app")
	own1 := filepath.Join(f.apps[1], "Other.app")
	foreign := filepath.Join(f.apps[0], "Foreign.app")
	user := filepath.Join(f.apps[0], "User.app")
	notBundle := filepath.Join(f.apps[0], "notes")
	for _, p := range []string{own0, own1, foreign, user, notBundle} {
		writeBundle(t, p, "x")
	}
	require.NoError(t, f.tagger.write(own0, bundleTag(bareA, own0)))
	require.NoError(t, f.tagger.write(own1, bundleTag(bareA, own1)))
	require.NoError(t, f.tagger.write(foreign, bundleTag(bareB, foreign)))
	require.NoError(t, f.tagger.write(notBundle, bundleTag(bareA, notBundle)))
	writeFile(t, filepath.Join(f.apps[0], "file.app"), "x", 0o600)

	kept := filepath.Join(f.apps[1], "Kept.app")
	writeBundle(t, kept, "x")
	require.NoError(t, f.tagger.write(kept, bundleTag(bareA, kept)))

	require.NoError(t, f.shelf.removeApps(layout{apps: append([]string{filepath.Join(t.TempDir(), "missing")}, f.apps...)}, bareA, map[string]bool{kept: true}))

	assert.DirExists(t, kept)

	assert.NoDirExists(t, own0)
	assert.NoDirExists(t, own1)
	assert.DirExists(t, foreign)
	assert.DirExists(t, user)
	assert.DirExists(t, notBundle)
}

func TestBundleTagOwner(t *testing.T) {
	bundle := filepath.Join("Applications", "Tool.app")
	testCases := []struct {
		name string
		tag  string
		want domain.Namespace
	}{
		{name: "tag naming this bundle is owned", tag: bundleTag(bareA, bundle), want: bareA},
		{name: "tag copied from another bundle is not owned", tag: bundleTag(bareA, filepath.Join("Applications", "Other.app"))},
		{name: "namespace-only tag is not owned", tag: bareA.String()},
		{name: "no tag is not owned", tag: ""},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, bundleTagOwner(tc.tag, bundle))
		})
	}
}

func TestShelf_CopiedBundle_IsNeverOwned(t *testing.T) {
	f := newFixture(t, goosDarwin)
	req, wd := appRequest(t, f, nsA)
	writeBundle(t, filepath.Join(wd, "Tool.app"), "v1")
	placed, err := f.shelf.placeApp(context.Background(), req, candidate{name: "Tool", target: filepath.Join(wd, "Tool.app")})
	require.NoError(t, err)
	copied := filepath.Join(f.apps[0], "Tool copy.app")
	writeBundle(t, copied, "user copy")
	tag, err := f.tagger.read(placed.location)
	require.NoError(t, err)
	require.NoError(t, f.tagger.write(copied, tag))

	holder, err := f.shelf.bundleHolder(copied)
	require.NoError(t, err)
	require.NoError(t, f.shelf.removeApps(layout{apps: f.apps}, bareA, nil))

	assert.Equal(t, domain.Namespace(""), holder.namespace)
	assert.NoDirExists(t, placed.location)
	assert.Equal(t, "user copy", readBundleVersion(t, copied))
}

func TestShelf_RemoveApps_Errors(t *testing.T) {
	f := newFixture(t, goosDarwin)
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	writeBundle(t, filepath.Join(f.apps[0], "Tool.app"), "x")

	assert.Error(t, f.shelf.removeApps(layout{apps: []string{file}}, bareA, nil))

	f.tagger.readErr = errors.New("boom")
	assert.NoError(t, f.shelf.removeApps(layout{apps: f.apps}, bareA, nil))
	assert.DirExists(t, filepath.Join(f.apps[0], "Tool.app"))
}

func TestShelf_PlaceApp_RequiresBundleSuffix(t *testing.T) {
	f := newFixture(t, goosDarwin)
	req, wd := appRequest(t, f, nsA)
	src := filepath.Join(wd, "Tool")
	writeBundle(t, src, "v1")

	got, err := f.shelf.placeApp(context.Background(), req, candidate{name: "Tool", target: src})

	require.NoError(t, err)
	assert.Equal(t, placement{refused: reasonWrongType}, got)
	assert.DirExists(t, src)
}

func TestShelf_SwapBundle_RollsBack(t *testing.T) {
	boom := errors.New("boom")

	testCases := []struct {
		name string
		fail func(from, to string) bool
	}{
		{
			name: "retiring the old bundle fails",
			fail: func(_, to string) bool { return strings.HasSuffix(to, retiredSuffix) },
		},
		{
			name: "moving the new bundle in fails",
			fail: func(from, _ string) bool {
				return strings.HasSuffix(from, stagedSuffix) && !strings.Contains(from, "namespaces")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, goosDarwin)
			req, wd := appRequest(t, f, nsA)
			src := filepath.Join(wd, "Tool.app")
			writeBundle(t, src, "v2")
			dest := filepath.Join(f.apps[0], "Tool.app")
			writeBundle(t, dest, "v1")
			require.NoError(t, f.tagger.write(dest, bundleTag(bareA, dest)))
			calls := 0
			f.shelf.rename = func(from, to string) error {
				calls++
				if calls < 5 && tc.fail(from, to) {
					return boom
				}
				return os.Rename(from, to)
			}

			_, err := f.shelf.placeApp(context.Background(), req, candidate{name: "Tool", target: src})

			require.ErrorIs(t, err, boom)
			assert.Equal(t, "v1", readBundleVersion(t, dest))
			assert.Equal(t, "v2", readBundleVersion(t, src))
			assert.NoDirExists(t, dest+retiredSuffix)
			assert.NoDirExists(t, dest+stagedSuffix)
		})
	}
}

func TestShelf_SwapBundle_ClearsLeftovers(t *testing.T) {
	f := newFixture(t, goosDarwin)
	req, wd := appRequest(t, f, nsA)
	src := filepath.Join(wd, "Tool.app")
	writeBundle(t, src, "v2")
	dest := filepath.Join(f.apps[0], "Tool.app")
	writeBundle(t, dest+retiredSuffix, "stale")
	writeBundle(t, dest+stagedSuffix, "stale")

	_, err := f.shelf.placeApp(context.Background(), req, candidate{name: "Tool", target: src})

	require.NoError(t, err)
	assert.Equal(t, "v2", readBundleVersion(t, dest))
	assert.NoDirExists(t, dest+retiredSuffix)
	assert.NoDirExists(t, dest+stagedSuffix)
}

func TestShelf_RemoveApps_RemoveError(t *testing.T) {
	requireUnixHost(t)
	if os.Geteuid() == 0 {
		t.Skip("root removes from read-only directories")
	}
	f := newFixture(t, goosDarwin)
	bundle := filepath.Join(f.apps[0], "Tool.app")
	writeBundle(t, bundle, "x")
	require.NoError(t, f.tagger.write(bundle, bundleTag(bareA, bundle)))
	require.NoError(t, os.Chmod(f.apps[0], 0o500))
	t.Cleanup(func() { _ = os.Chmod(f.apps[0], 0o750) })

	assert.Error(t, f.shelf.removeApps(layout{apps: f.apps}, bareA, nil))
}

func TestShelf_EnclosingBundleOwner(t *testing.T) {
	f := newFixture(t, goosDarwin)
	tagged := filepath.Join(f.apps[0], "Tool.app")
	writeBundle(t, tagged, "x")
	require.NoError(t, f.tagger.write(tagged, bundleTag(bareA, tagged)))
	untagged := filepath.Join(f.apps[0], "Other.app")
	writeBundle(t, untagged, "x")

	assert.Equal(t, bareA, f.shelf.enclosingBundleOwner(filepath.Join(tagged, "Contents", "MacOS", "tool")))
	assert.Equal(t, domain.Namespace(""), f.shelf.enclosingBundleOwner(filepath.Join(untagged, "Contents", "tool")))

	f.tagger.readErr = errors.New("boom")
	assert.Equal(t, domain.Namespace(""), f.shelf.enclosingBundleOwner(filepath.Join(tagged, "Contents", "tool")))
}

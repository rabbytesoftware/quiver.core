package desktop

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
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/fsguard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/ownership"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/platform"
)

func TestBundleName(t *testing.T) {
	assert.Equal(t, "Tool.app", bundleName("Tool"))
	assert.Equal(t, "Tool.app", bundleName("Tool.app"))
}

func TestPlacer_PlaceApp_MovesAndTags(t *testing.T) {
	f := newFixture(t, platform.GOOSDarwin)
	req, wd := appRequest(t, f, mocks.NsA)
	src := filepath.Join(wd, "Tool.app")
	mocks.WriteBundle(t, src, "v1")
	dest := filepath.Join(f.Apps[0], "Tool.app")

	got, err := f.placer.placeApp(context.Background(), req, models.Candidate{Name: "Tool", Target: src})

	require.NoError(t, err)
	assert.Equal(t, models.Placement{Location: dest}, got)
	assert.NoDirExists(t, src)
	assert.Equal(t, "v1", mocks.ReadBundleVersion(t, dest))
	tag, err := f.Tagger.Read(dest)
	require.NoError(t, err)
	assert.Equal(t, "github.com/acme/tool\nTool.app", tag)
	assert.Equal(t, map[string]string{src: dest}, req.Moved)
	assert.NoDirExists(t, dest+fsguard.StagedSuffix)
}

func TestPlacer_PlaceApp_ReplacesOwnedBundle(t *testing.T) {
	f := newFixture(t, platform.GOOSDarwin)
	req1, wd1 := appRequest(t, f, mocks.NsA)
	req2, wd2 := appRequest(t, f, mocks.NsA2)
	mocks.WriteBundle(t, filepath.Join(wd1, "Tool.app"), "v1")
	mocks.WriteBundle(t, filepath.Join(wd2, "Tool.app"), "v2")

	_, err := f.placer.placeApp(context.Background(), req1, models.Candidate{Name: "Tool", Target: filepath.Join(wd1, "Tool.app")})
	require.NoError(t, err)
	got, err := f.placer.placeApp(context.Background(), req2, models.Candidate{Name: "Tool", Target: filepath.Join(wd2, "Tool.app")})
	require.NoError(t, err)

	dest := filepath.Join(f.Apps[0], "Tool.app")
	assert.Equal(t, models.Placement{Location: dest}, got)
	assert.Equal(t, "v2", mocks.ReadBundleVersion(t, dest))
}

func TestPlacer_PlaceApp_ReplacesOwnedBundleWhereItLives(t *testing.T) {
	f := newFixture(t, platform.GOOSDarwin)
	req, wd := appRequest(t, f, mocks.NsA)
	mocks.WriteBundle(t, filepath.Join(wd, "Tool.app"), "v2")
	existing := filepath.Join(f.Apps[1], "Tool.app")
	mocks.WriteBundle(t, existing, "v1")
	require.NoError(t, f.Tagger.Write(existing, ownership.BundleTag(mocks.BareA, existing)))

	got, err := f.placer.placeApp(context.Background(), req, models.Candidate{Name: "Tool", Target: filepath.Join(wd, "Tool.app")})

	require.NoError(t, err)
	assert.Equal(t, models.Placement{Location: existing}, got)
	assert.Equal(t, "v2", mocks.ReadBundleVersion(t, existing))
	assert.NoDirExists(t, filepath.Join(f.Apps[0], "Tool.app"))
}

func TestPlacer_PlaceApp_Refusals(t *testing.T) {
	testCases := []struct {
		name     string
		owner    domain.Namespace
		existing bool
		source   string
		want     string
	}{
		{name: "foreign bundle", existing: true, owner: mocks.BareB, source: "dir", want: "owned by github.com/other/thing"},
		{name: "user bundle", existing: true, source: "dir", want: models.ReasonUnmanaged},
		{name: "missing source", source: "none", want: models.ReasonNotFound},
		{name: "file source", source: "file", want: models.ReasonWrongType},
		{name: "unreadable owner tag counts as unmanaged", existing: true, owner: "unreadable", source: "dir", want: models.ReasonUnmanaged},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, platform.GOOSDarwin)
			req, wd := appRequest(t, f, mocks.NsA)
			src := filepath.Join(wd, "Tool.app")
			dest := filepath.Join(f.Apps[0], "Tool.app")
			if tc.source == "dir" {
				mocks.WriteBundle(t, src, "new")
			}
			if tc.source == "file" {
				mocks.WriteFile(t, src, "x", 0o600)
			}
			if tc.existing {
				mocks.WriteBundle(t, dest, "user")
			}
			if tc.owner != "" && tc.owner != "unreadable" {
				require.NoError(t, f.Tagger.Write(dest, ownership.BundleTag(tc.owner, dest)))
			}
			if tc.owner == "unreadable" {
				f.Tagger.ReadErr = errors.New("boom")
			}

			got, err := f.placer.placeApp(context.Background(), req, models.Candidate{Name: "Tool", Target: src})

			require.NoError(t, err)
			assert.Equal(t, models.Placement{Refused: tc.want}, got)
			if tc.existing {
				assert.Equal(t, "user", mocks.ReadBundleVersion(t, dest))
			}
		})
	}
}

func TestPlacer_PlaceApp_AlreadyMovedIsIdempotent(t *testing.T) {
	f := newFixture(t, platform.GOOSDarwin)
	req, wd := appRequest(t, f, mocks.NsA)
	src := filepath.Join(wd, "Tool.app")
	mocks.WriteBundle(t, src, "v1")

	first, err := f.placer.placeApp(context.Background(), req, models.Candidate{Name: "Tool", Target: src})
	require.NoError(t, err)
	req.Moved = map[string]string{}
	second, err := f.placer.placeApp(context.Background(), req, models.Candidate{Name: "Tool", Target: src})
	require.NoError(t, err)

	assert.Equal(t, first, second)
	assert.Equal(t, map[string]string{src: first.Location}, req.Moved)
}

func TestPlacer_PlaceApp_FallsBackToNextWritableDir(t *testing.T) {
	f := newFixture(t, platform.GOOSDarwin)
	f.Apps = []string{mocks.ReadOnlyDir(t), f.Apps[1]}
	req, wd := appRequest(t, f, mocks.NsA)
	mocks.WriteBundle(t, filepath.Join(wd, "Tool.app"), "v1")

	got, err := f.placer.placeApp(context.Background(), req, models.Candidate{Name: "Tool", Target: filepath.Join(wd, "Tool.app")})

	require.NoError(t, err)
	assert.Equal(t, models.Placement{Location: filepath.Join(f.Apps[1], "Tool.app")}, got)
}

func TestPlacer_PlaceApp_NoWritableDir(t *testing.T) {
	f := newFixture(t, platform.GOOSDarwin)
	f.Apps = []string{mocks.ReadOnlyDir(t)}
	req, wd := appRequest(t, f, mocks.NsA)
	src := filepath.Join(wd, "Tool.app")
	mocks.WriteBundle(t, src, "v1")

	_, err := f.placer.placeApp(context.Background(), req, models.Candidate{Name: "Tool", Target: src})

	require.Error(t, err)
	assert.DirExists(t, src)
}

func TestPlacer_PlaceApp_TaggerErrors(t *testing.T) {
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
			f := newFixture(t, platform.GOOSDarwin)
			req, wd := appRequest(t, f, mocks.NsA)
			src := filepath.Join(wd, "Tool.app")
			mocks.WriteBundle(t, src, "v1")
			if tc.existing {
				mocks.WriteBundle(t, filepath.Join(f.Apps[0], "Tool.app"), "x")
				require.NoError(t, f.Tagger.Write(filepath.Join(f.Apps[0], "Tool.app"), ownership.BundleTag(mocks.BareA, filepath.Join(f.Apps[0], "Tool.app"))))
			}
			f.Tagger.ReadErr = tc.readErr
			f.Tagger.WriteErr = tc.writeErr

			_, err := f.placer.placeApp(context.Background(), req, models.Candidate{Name: "Tool", Target: src})

			require.ErrorIs(t, err, boom)
			assert.Equal(t, "v1", mocks.ReadBundleVersion(t, src))
		})
	}
}

func TestPlacer_PlaceApp_LstatError(t *testing.T) {
	mocks.RequireUnixHost(t)
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	f := newFixture(t, platform.GOOSDarwin)
	f.Apps = []string{file}
	req, wd := appRequest(t, f, mocks.NsA)
	mocks.WriteBundle(t, filepath.Join(wd, "Tool.app"), "v1")

	_, err := f.placer.placeApp(context.Background(), req, models.Candidate{Name: "Tool", Target: filepath.Join(wd, "Tool.app")})

	require.Error(t, err)
}

func TestPlacer_RemoveApps(t *testing.T) {
	f := newFixture(t, platform.GOOSDarwin)
	own0 := filepath.Join(f.Apps[0], "Tool.app")
	own1 := filepath.Join(f.Apps[1], "Other.app")
	foreign := filepath.Join(f.Apps[0], "Foreign.app")
	user := filepath.Join(f.Apps[0], "User.app")
	notBundle := filepath.Join(f.Apps[0], "notes")
	for _, p := range []string{own0, own1, foreign, user, notBundle} {
		mocks.WriteBundle(t, p, "x")
	}
	require.NoError(t, f.Tagger.Write(own0, ownership.BundleTag(mocks.BareA, own0)))
	require.NoError(t, f.Tagger.Write(own1, ownership.BundleTag(mocks.BareA, own1)))
	require.NoError(t, f.Tagger.Write(foreign, ownership.BundleTag(mocks.BareB, foreign)))
	require.NoError(t, f.Tagger.Write(notBundle, ownership.BundleTag(mocks.BareA, notBundle)))
	mocks.WriteFile(t, filepath.Join(f.Apps[0], "file.app"), "x", 0o600)

	kept := filepath.Join(f.Apps[1], "Kept.app")
	mocks.WriteBundle(t, kept, "x")
	require.NoError(t, f.Tagger.Write(kept, ownership.BundleTag(mocks.BareA, kept)))

	require.NoError(t, f.placer.removeApps(platform.Layout{Apps: append([]string{filepath.Join(t.TempDir(), "missing")}, f.Apps...)}, mocks.BareA, map[string]bool{kept: true}))

	assert.DirExists(t, kept)

	assert.NoDirExists(t, own0)
	assert.NoDirExists(t, own1)
	assert.DirExists(t, foreign)
	assert.DirExists(t, user)
	assert.DirExists(t, notBundle)
}

func TestPlacer_CopiedBundle_IsNeverOwned(t *testing.T) {
	f := newFixture(t, platform.GOOSDarwin)
	req, wd := appRequest(t, f, mocks.NsA)
	mocks.WriteBundle(t, filepath.Join(wd, "Tool.app"), "v1")
	placed, err := f.placer.placeApp(context.Background(), req, models.Candidate{Name: "Tool", Target: filepath.Join(wd, "Tool.app")})
	require.NoError(t, err)
	copied := filepath.Join(f.Apps[0], "Tool copy.app")
	mocks.WriteBundle(t, copied, "user copy")
	tag, err := f.Tagger.Read(placed.Location)
	require.NoError(t, err)
	require.NoError(t, f.Tagger.Write(copied, tag))

	holder, err := f.placer.bundles.Holder(copied)
	require.NoError(t, err)
	require.NoError(t, f.placer.removeApps(platform.Layout{Apps: f.Apps}, mocks.BareA, nil))

	assert.Equal(t, domain.Namespace(""), holder.Namespace)
	assert.NoDirExists(t, placed.Location)
	assert.Equal(t, "user copy", mocks.ReadBundleVersion(t, copied))
}

func TestPlacer_RemoveApps_Errors(t *testing.T) {
	f := newFixture(t, platform.GOOSDarwin)
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	mocks.WriteBundle(t, filepath.Join(f.Apps[0], "Tool.app"), "x")

	assert.Error(t, f.placer.removeApps(platform.Layout{Apps: []string{file}}, mocks.BareA, nil))

	f.Tagger.ReadErr = errors.New("boom")
	assert.NoError(t, f.placer.removeApps(platform.Layout{Apps: f.Apps}, mocks.BareA, nil))
	assert.DirExists(t, filepath.Join(f.Apps[0], "Tool.app"))
}

func TestPlacer_PlaceApp_RequiresBundleSuffix(t *testing.T) {
	f := newFixture(t, platform.GOOSDarwin)
	req, wd := appRequest(t, f, mocks.NsA)
	src := filepath.Join(wd, "Tool")
	mocks.WriteBundle(t, src, "v1")

	got, err := f.placer.placeApp(context.Background(), req, models.Candidate{Name: "Tool", Target: src})

	require.NoError(t, err)
	assert.Equal(t, models.Placement{Refused: models.ReasonWrongType}, got)
	assert.DirExists(t, src)
}

func TestPlacer_SwapBundle_RollsBack(t *testing.T) {
	boom := errors.New("boom")

	testCases := []struct {
		name string
		fail func(from, to string) bool
	}{
		{
			name: "retiring the old bundle fails",
			fail: func(_, to string) bool { return strings.HasSuffix(to, fsguard.RetiredSuffix) },
		},
		{
			name: "moving the new bundle in fails",
			fail: func(from, _ string) bool {
				return strings.HasSuffix(from, fsguard.StagedSuffix) && !strings.Contains(from, "namespaces")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, platform.GOOSDarwin)
			req, wd := appRequest(t, f, mocks.NsA)
			src := filepath.Join(wd, "Tool.app")
			mocks.WriteBundle(t, src, "v2")
			dest := filepath.Join(f.Apps[0], "Tool.app")
			mocks.WriteBundle(t, dest, "v1")
			require.NoError(t, f.Tagger.Write(dest, ownership.BundleTag(mocks.BareA, dest)))
			calls := 0
			f.placer.host.Rename = func(from, to string) error {
				calls++
				if calls < 5 && tc.fail(from, to) {
					return boom
				}
				return os.Rename(from, to)
			}

			_, err := f.placer.placeApp(context.Background(), req, models.Candidate{Name: "Tool", Target: src})

			require.ErrorIs(t, err, boom)
			assert.Equal(t, "v1", mocks.ReadBundleVersion(t, dest))
			assert.Equal(t, "v2", mocks.ReadBundleVersion(t, src))
			assert.NoDirExists(t, dest+fsguard.RetiredSuffix)
			assert.NoDirExists(t, dest+fsguard.StagedSuffix)
		})
	}
}

func TestPlacer_SwapBundle_ClearsLeftovers(t *testing.T) {
	f := newFixture(t, platform.GOOSDarwin)
	req, wd := appRequest(t, f, mocks.NsA)
	src := filepath.Join(wd, "Tool.app")
	mocks.WriteBundle(t, src, "v2")
	dest := filepath.Join(f.Apps[0], "Tool.app")
	mocks.WriteBundle(t, dest+fsguard.RetiredSuffix, "stale")
	mocks.WriteBundle(t, dest+fsguard.StagedSuffix, "stale")

	_, err := f.placer.placeApp(context.Background(), req, models.Candidate{Name: "Tool", Target: src})

	require.NoError(t, err)
	assert.Equal(t, "v2", mocks.ReadBundleVersion(t, dest))
	assert.NoDirExists(t, dest+fsguard.RetiredSuffix)
	assert.NoDirExists(t, dest+fsguard.StagedSuffix)
}

func TestPlacer_RemoveApps_RemoveError(t *testing.T) {
	mocks.RequireUnixHost(t)
	if os.Geteuid() == 0 {
		t.Skip("root removes from read-only directories")
	}
	f := newFixture(t, platform.GOOSDarwin)
	bundle := filepath.Join(f.Apps[0], "Tool.app")
	mocks.WriteBundle(t, bundle, "x")
	require.NoError(t, f.Tagger.Write(bundle, ownership.BundleTag(mocks.BareA, bundle)))
	require.NoError(t, os.Chmod(f.Apps[0], 0o500))
	t.Cleanup(func() { _ = os.Chmod(f.Apps[0], 0o750) })

	assert.Error(t, f.placer.removeApps(platform.Layout{Apps: f.Apps}, mocks.BareA, nil))
}

func TestRequireBundle_InspectError(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	_, err := requireBundle(filepath.Join(file, "Tool.app"))

	require.Error(t, err)
}

func TestProbeWritable_UnusableDirIsNotWritable(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	assert.False(t, probeWritable(file))
}

func TestPlacer_SwapBundle_MoveFailure(t *testing.T) {
	f := newFixture(t, platform.GOOSDarwin)
	f.placer.host.Rename = func(string, string) error { return os.ErrPermission }
	src := filepath.Join(t.TempDir(), "Tool.app")
	mocks.WriteBundle(t, src, "v1")

	err := f.placer.swapBundle(context.Background(), src, filepath.Join(t.TempDir(), "Tool.app"), mocks.BareA)

	require.ErrorIs(t, err, os.ErrPermission)
}

func TestClearPaths_RemoveFailure(t *testing.T) {
	mocks.RequireUnixHost(t)
	if os.Geteuid() == 0 {
		t.Skip("root removes from read-only directories")
	}
	dir := t.TempDir()
	stuck := filepath.Join(dir, "stuck")
	mocks.WriteFile(t, filepath.Join(stuck, "child"), "x", 0o600)
	require.NoError(t, os.Chmod(stuck, 0o500))
	t.Cleanup(func() { _ = os.Chmod(stuck, 0o750) })

	require.Error(t, clearPaths(stuck))

	f := newFixture(t, platform.GOOSDarwin)
	dest := filepath.Join(dir, "Tool.app")
	require.Error(t, f.placer.swapBundle(context.Background(), filepath.Join(dir, "src.app"), dest, mocks.BareA))
}

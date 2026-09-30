package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/store"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	manifoldresolver "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
	"github.com/rabbytesoftware/quiver.core/internal/mocks"
)

// stableTarget is what selectorSnapshot's stable channel points at.
var stableTarget = domain.Available{Ref: "v2.0.0", Commit: "c200"}

func realVault(t *testing.T) vault.Vault {
	t.Helper()
	v, err := vault.New(t.TempDir(), t.TempDir(), time.Hour)
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })
	return v
}

func cachedAt(
	t *testing.T,
	v vault.Vault,
	ns domain.Namespace,
	release domain.Available,
) {
	t.Helper()
	require.NoError(t, v.PutArrow(context.Background(), ns, vault.ManifestFile{
		Content: []byte("cached"), Filename: "ARROW.md", Ref: release.Ref, Commit: release.Commit,
	}))
}

func countingFetches(
	snap domain.RefSnapshot,
	fetchErr error,
	fetched *int,
) *mocks.Manifold {
	return &mocks.Manifold{
		SnapshotResult:   snap,
		ParseArrowResult: &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "crowbar"}},
		ResolveArrowAtCommitFn: func(_ context.Context, ns domain.Namespace, _, _ string) (*domain.Arrow, []byte, string, error) {
			*fetched++
			if fetchErr != nil {
				return nil, nil, "", fetchErr
			}
			return &domain.Arrow{Namespace: ns}, []byte("fetched"), "ARROW.md", nil
		},
	}
}

// An add reuses the build the vault holds for the very commit it records —
// what discovery drafted, filed at the release tag — and reads the host only
// when no copy names that commit, so a moved tag is never served stale.
func TestResolveInstall_ReusesTheVaultCopyOfTheTargetCommit(t *testing.T) {
	testCases := []struct {
		name        string
		cacheAt     domain.Namespace
		release     domain.Available
		wantFetches int
	}{
		{name: "discovery's build at the release tag", cacheAt: selectorBare.WithRef("v2.0.0"), release: stableTarget},
		{name: "the identity's own copy", cacheAt: selectorBare.WithRef("stable"), release: domain.Available{Ref: "v2.0.0", Commit: "C200"}},
		{name: "a copy of the commit the tag moved from", cacheAt: selectorBare.WithRef("v2.0.0"), release: domain.Available{Ref: "v2.0.0", Commit: "c199"}, wantFetches: 1},
		{name: "another tag of the same commit", cacheAt: selectorBare.WithRef("stable"), release: domain.Available{Ref: "v1.9.0", Commit: "c200"}, wantFetches: 1},
		{name: "a copy of an unknown release", cacheAt: selectorBare.WithRef("v2.0.0"), wantFetches: 1},
		{name: "nothing cached", wantFetches: 1},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			v := realVault(t)
			if tc.cacheAt != "" {
				cachedAt(t, v, tc.cacheAt, tc.release)
			}
			fetches := 0
			r := newTestReaderWithVaultManifold(t, v, countingFetches(selectorSnapshot(), nil, &fetches))

			identity, arrow, err := r.ResolveInstall(context.Background(), selectorBare, store.CacheWhenAbsent(absent))

			require.NoError(t, err)
			assert.Equal(t, tc.wantFetches, fetches)
			assert.Equal(t, selectorBare.WithRef("stable"), identity)
			assert.Equal(t, identity, arrow.Namespace)
			cached, err := v.GetArrow(context.Background(), identity)
			require.NoError(t, err)
			assert.Equal(t, stableTarget.Ref, cached.Ref, "the add records the release it read")
			assert.Equal(t, stableTarget.Commit, cached.Commit)
		})
	}
}

// A preview of a discovered arrow reads discovery's build and fetches nothing.
func TestResolveManifest_UncataloguedPreview_ReusesTheBuildAtTheTarget(t *testing.T) {
	v := realVault(t)
	cachedAt(t, v, selectorBare.WithRef("v2.0.0"), stableTarget)
	fetches := 0
	r := newTestReaderWithVaultManifold(t, v, countingFetches(selectorSnapshot(), nil, &fetches))

	arrow, err := r.ResolveManifest(context.Background(), selectorBare)

	require.NoError(t, err)
	assert.Zero(t, fetches)
	assert.Equal(t, selectorBare.WithRef("stable"), arrow.Namespace)
	assert.Equal(t, domain.Resolved{Ref: "v2.0.0", Commit: "c200", Fingerprint: "c200"}, arrow.Resolved)
}

// A target that definitively holds no manifest is recorded at its ref for
// its commit. A pin's ref is its identity, whose entry is recorded only while
// it holds no manifest: a row's installed manifest is never replaced.
func TestResolveInstall_AbsentTargetIsRecordedAtItsRef(t *testing.T) {
	testCases := []struct {
		name       string
		ns         domain.Namespace
		cached     bool
		wantMarker domain.Namespace
	}{
		{name: "a channel's release", ns: selectorBare.WithRef("stable"), wantMarker: selectorBare.WithRef("v2.0.0")},
		{name: "a pin nothing is cached for", ns: selectorBare.WithRef("v2.0.0"), wantMarker: selectorBare.WithRef("v2.0.0")},
		{name: "a pin whose identity holds a manifest", ns: selectorBare.WithRef("v2.0.0"), cached: true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			v := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
			if tc.cached {
				v = &mocks.Vault{GetArrowFile: vault.ManifestFile{Content: []byte("installed"), Filename: "ARROW.md"}}
			}
			fetches := 0
			r := newTestReaderWithVaultManifold(t, v, countingFetches(selectorSnapshot(), manifoldresolver.ErrNotFound, &fetches))

			_, _, err := r.ResolveInstall(context.Background(), tc.ns)

			require.ErrorIs(t, err, apperrors.ErrNotFound)
			if tc.wantMarker == "" {
				assert.Zero(t, v.PutArrowNotFoundCalls)
				return
			}
			assert.Equal(t, []domain.Namespace{tc.wantMarker}, v.PutArrowNotFoundNamespaces)
			assert.Equal(t, []string{"c200"}, v.PutArrowNotFoundCommits)
		})
	}
}

// Once a fetch found no manifest at a commit, reading that commit again —
// the details of a repository an add just failed on — asks the host nothing
// about it, until the tag moves. A refless read may still try other channels.
func TestResolveManifest_KnownAbsentTarget_AsksTheHostNothing(t *testing.T) {
	testCases := []struct {
		name        string
		ns          domain.Namespace
		absentAt    string
		wantFetches int
	}{
		{name: "a pin", ns: selectorBare.WithRef("v2.0.0"), absentAt: "c200"},
		{name: "the default channel's release", ns: selectorBare, absentAt: "c200"},
		{name: "a tag that moved since", ns: selectorBare, absentAt: "c199", wantFetches: 1},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			v := realVault(t)
			require.NoError(t, v.PutArrowNotFound(context.Background(), selectorBare.WithRef("v2.0.0"), tc.absentAt))
			fetchedTarget := 0
			m := countingFetches(selectorSnapshot(), manifoldresolver.ErrNotFound, new(int))
			m.ResolveArrowAtCommitFn = func(_ context.Context, _ domain.Namespace, ref, _ string) (*domain.Arrow, []byte, string, error) {
				if ref == stableTarget.Ref {
					fetchedTarget++
				}
				return nil, nil, "", manifoldresolver.ErrNotFound
			}
			r := newTestReaderWithVaultManifold(t, v, m)

			_, err := r.ResolveManifest(context.Background(), tc.ns)

			require.ErrorIs(t, err, apperrors.ErrNotFound)
			assert.Equal(t, tc.wantFetches, fetchedTarget)
		})
	}
}

// A version check decides from the snapshot alone: it fetches no manifest,
// and holds back only a target a fetch already found empty at that commit.
func TestCheckDrift_OffersFromTheSnapshotAlone(t *testing.T) {
	row := domain.Arrow{
		Namespace:    selectorBare.WithRef("stable"),
		SelectorKind: domain.SelectorChannel,
		Resolved:     domain.Resolved{Ref: "v1.3.0", Commit: "c130"},
	}
	testCases := []struct {
		name          string
		absentCommit  string
		wantAvailable *domain.Available
	}{
		{name: "nothing known", wantAvailable: &stableTarget},
		{name: "known empty at the target commit", absentCommit: "c200"},
		{name: "known empty at another commit", absentCommit: "c199", wantAvailable: &stableTarget},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			v := realVault(t)
			if tc.absentCommit != "" {
				require.NoError(t, v.PutArrowNotFound(context.Background(), selectorBare.WithRef("v2.0.0"), tc.absentCommit))
			}
			fetches := 0
			r := newTestReaderWithVaultManifold(t, v, countingFetches(selectorSnapshot(), nil, &fetches))

			available, ok := r.CheckDrift(context.Background(), row)

			require.True(t, ok)
			assert.Equal(t, tc.wantAvailable, available)
			assert.Zero(t, fetches, "a version check reads no manifest")
		})
	}
}

// A periodic sweep costs one ref listing per row and no manifest fetch.
func TestCheckDrift_ASweepListsRefsOnly(t *testing.T) {
	const rows = 5
	fetches := 0
	m := countingFetches(selectorSnapshot(), nil, &fetches)
	r := newTestReaderWithVaultManifold(t, realVault(t), m)

	offered := 0
	for i := range rows {
		row := domain.Arrow{
			Namespace:    domain.Namespace("github.com/char2cs/crowbar" + string(rune('a'+i))).WithRef("stable"),
			SelectorKind: domain.SelectorChannel,
			Resolved:     domain.Resolved{Ref: "v1.3.0", Commit: "c130"},
		}
		if available, ok := r.CheckDrift(context.Background(), row); ok && available != nil {
			offered++
		}
	}

	assert.Equal(t, rows, offered)
	assert.Equal(t, rows, m.FreshSnapshotCalls)
	assert.Zero(t, fetches)
}

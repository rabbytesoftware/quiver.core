package store_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/store"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	manifoldresolver "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
	"github.com/rabbytesoftware/quiver.core/internal/mocks"
)

const (
	selectorBare = domain.Namespace("github.com/char2cs/crowbar")
	headSHA      = "abcdef1234567"
)

func selectorSnapshot() domain.RefSnapshot {
	return domain.RefSnapshot{
		Tags: map[string]string{
			"v1.0.0":  "c100",
			"v1.2.0":  "c120",
			"v1.3.0":  "c130",
			"v2.0.0":  "c200",
			"nightly": "cnightly",
		},
		Branches: map[string]string{"main": "cmain"},
		Head:     "main",
	}
}

type commitFetch struct {
	ns     domain.Namespace
	ref    string
	commit string
}

func selectorManifold(
	snap domain.RefSnapshot,
	fetched *[]commitFetch,
) *mocks.Manifold {
	return &mocks.Manifold{
		SnapshotResult: snap,
		ResolveArrowAtCommitFn: func(
			_ context.Context,
			ns domain.Namespace,
			ref string,
			commit string,
		) (*domain.Arrow, []byte, string, error) {
			*fetched = append(*fetched, commitFetch{ns: ns, ref: ref, commit: commit})
			return &domain.Arrow{
				Namespace: ns,
				ArrowMeta: domain.ArrowMeta{Name: "crowbar"},
			}, []byte("raw"), "ARROW.md", nil
		},
	}
}

func TestResolveInstall_Selectors(t *testing.T) {
	testCases := []struct {
		name         string
		ns           domain.Namespace
		snap         domain.RefSnapshot
		wantIdentity domain.Namespace
		wantKind     domain.SelectorKind
		wantResolved domain.Resolved
	}{
		{
			name:         "refless follows the default channel",
			ns:           selectorBare,
			snap:         selectorSnapshot(),
			wantIdentity: selectorBare.WithRef("stable"),
			wantKind:     domain.SelectorOrderedChannel,
			wantResolved: domain.Resolved{Ref: "v2.0.0", Commit: "c200", Fingerprint: "c200"},
		},
		{
			name: "refless with no tags follows the head branch",
			ns:   selectorBare,
			snap: domain.RefSnapshot{
				Branches: map[string]string{"main": "cmain"},
				Head:     "main",
			},
			wantIdentity: selectorBare.WithRef("main"),
			wantKind:     domain.SelectorBranchChannel,
			wantResolved: domain.Resolved{Ref: "main", Commit: "cmain", Fingerprint: "cmain"},
		},
		{
			name:         "stable channel keeps its identity",
			ns:           selectorBare.WithRef("stable"),
			snap:         selectorSnapshot(),
			wantIdentity: selectorBare.WithRef("stable"),
			wantKind:     domain.SelectorOrderedChannel,
			wantResolved: domain.Resolved{Ref: "v2.0.0", Commit: "c200", Fingerprint: "c200"},
		},
		{
			name:         "rolling tag is a channel at the tag's commit",
			ns:           selectorBare.WithRef("nightly"),
			snap:         selectorSnapshot(),
			wantIdentity: selectorBare.WithRef("nightly"),
			wantKind:     domain.SelectorPointerChannel,
			wantResolved: domain.Resolved{Ref: "nightly", Commit: "cnightly", Fingerprint: "cnightly"},
		},
		{
			name:         "exact tag is a pin",
			ns:           selectorBare.WithRef("v1.2.0"),
			snap:         selectorSnapshot(),
			wantIdentity: selectorBare.WithRef("v1.2.0"),
			wantKind:     domain.SelectorTagPin,
			wantResolved: domain.Resolved{Ref: "v1.2.0", Commit: "c120", Fingerprint: "c120"},
		},
		{
			name:         "branch is a pin",
			ns:           selectorBare.WithRef("main"),
			snap:         selectorSnapshot(),
			wantIdentity: selectorBare.WithRef("main"),
			wantKind:     domain.SelectorBranchPin,
			wantResolved: domain.Resolved{Ref: "main", Commit: "cmain", Fingerprint: "cmain"},
		},
		{
			name:         "glob is a constraint at its highest match within bounds",
			ns:           selectorBare.WithRef("v1.*"),
			snap:         selectorSnapshot(),
			wantIdentity: selectorBare.WithRef("v1.*"),
			wantKind:     domain.SelectorConstraint,
			wantResolved: domain.Resolved{Ref: "v1.3.0", Commit: "c130", Fingerprint: "c130"},
		},
		{
			name:         "unlisted hex is a commit",
			ns:           selectorBare.WithRef(headSHA),
			snap:         selectorSnapshot(),
			wantIdentity: selectorBare.WithRef(headSHA),
			wantKind:     domain.SelectorCommit,
			wantResolved: domain.Resolved{Ref: headSHA, Commit: headSHA, Fingerprint: headSHA},
		},
		{
			name:         "a commit spelled in upper case is the lower-case identity",
			ns:           selectorBare.WithRef(strings.ToUpper(headSHA)),
			snap:         selectorSnapshot(),
			wantIdentity: selectorBare.WithRef(headSHA),
			wantKind:     domain.SelectorCommit,
			wantResolved: domain.Resolved{Ref: headSHA, Commit: headSHA, Fingerprint: headSHA},
		},
		{
			name: "escaped tag named like a channel is a pin on the short ref",
			ns:   selectorBare.WithRef("refs/tags/stable"),
			snap: domain.RefSnapshot{
				Tags: map[string]string{"stable": "cst", "v1.0.0": "c100"},
			},
			wantIdentity: selectorBare.WithRef("refs/tags/stable"),
			wantKind:     domain.SelectorTagPin,
			wantResolved: domain.Resolved{Ref: "stable", Commit: "cst", Fingerprint: "cst"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var fetched []commitFetch
			r := newTestReaderWithVaultManifold(t, nil, selectorManifold(tc.snap, &fetched))

			identity, arrow, err := r.ResolveInstall(context.Background(), tc.ns)
			require.NoError(t, err)
			require.NotNil(t, arrow)

			assert.Equal(t, tc.wantIdentity, identity)
			assert.Equal(t, tc.wantIdentity, arrow.Namespace)
			assert.Equal(t, tc.wantKind, arrow.SelectorKind)
			assert.Equal(t, tc.wantResolved, arrow.Resolved)
			assert.Nil(t, arrow.Available)
			assert.Equal(t, []commitFetch{{ns: tc.wantIdentity, ref: tc.wantResolved.Ref, commit: tc.wantResolved.Commit}}, fetched)
		})
	}
}

func TestResolveInstall_InstalledSelectorIsCurrent(t *testing.T) {
	selectors := []string{"stable", "nightly", "v1.2.0", "main", "v1.*", headSHA, "refs/tags/v1.0.0"}
	for _, selector := range selectors {
		t.Run(selector, func(t *testing.T) {
			var fetched []commitFetch
			r := newTestReaderWithVaultManifold(t, nil, selectorManifold(selectorSnapshot(), &fetched))

			_, arrow, err := r.ResolveInstall(context.Background(), selectorBare.WithRef(selector))
			require.NoError(t, err)

			available, ok := r.CheckDrift(context.Background(), *arrow)
			require.True(t, ok)
			assert.Nil(t, available)
		})
	}
}

func absent(context.Context, domain.Namespace) (bool, error) { return false, nil }

func TestResolveInstall_Cache(t *testing.T) {
	existsErr := errors.New("event store down")

	testCases := []struct {
		name      string
		opts      []store.InstallOption
		wantOps   bool
		wantErr   error
		wantAsked bool
	}{
		{name: "no cache option never touches the vault"},
		{
			name:      "absent identity is cached under the identity",
			opts:      []store.InstallOption{store.CacheWhenAbsent(absent)},
			wantOps:   true,
			wantAsked: true,
		},
		{
			name: "installed identity keeps its cache",
			opts: []store.InstallOption{store.CacheWhenAbsent(func(context.Context, domain.Namespace) (bool, error) {
				return true, nil
			})},
			wantAsked: true,
		},
		{
			name: "existence check failure touches nothing",
			opts: []store.InstallOption{store.CacheWhenAbsent(func(context.Context, domain.Namespace) (bool, error) {
				return false, existsErr
			})},
			wantErr:   existsErr,
			wantAsked: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var fetched []commitFetch
			v := &mocks.Vault{}
			r := newTestReaderWithVaultManifold(t, v, selectorManifold(selectorSnapshot(), &fetched))

			identity, arrow, err := r.ResolveInstall(context.Background(), selectorBare, tc.opts...)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				assert.Nil(t, arrow)
				assert.Empty(t, v.ArrowOps)
				return
			}
			require.NoError(t, err)

			if !tc.wantOps {
				assert.Empty(t, v.ArrowOps)
				return
			}
			assert.Equal(t, []string{"delete " + identity.String(), "put " + identity.String()}, v.ArrowOps)
			require.Len(t, v.PutArrowFiles, 1)
			assert.Equal(t, []byte("raw"), v.PutArrowFiles[0].Content)
			assert.Equal(t, "ARROW.md", v.PutArrowFiles[0].Filename)
			assert.NotNil(t, v.PutArrowFiles[0].Meta)
		})
	}
}

func TestResolveInstall_Errors(t *testing.T) {
	fetchErr := errors.New("boom")

	testCases := []struct {
		name    string
		ns      domain.Namespace
		m       *mocks.Manifold
		v       *mocks.Vault
		wantErr error
	}{
		{
			name:    "unknown selector is an invalid namespace",
			ns:      selectorBare.WithRef("no-such-ref"),
			m:       &mocks.Manifold{SnapshotResult: selectorSnapshot()},
			wantErr: apperrors.ErrInvalidNamespace,
		},
		{
			name: "doubled slash ref component is an invalid namespace",
			ns:   selectorBare.WithRef("feat//x"),
			m: &mocks.Manifold{SnapshotResult: domain.RefSnapshot{
				Branches: map[string]string{"feat//x": "cfeat", "main": "cmain"},
				Head:     "main",
			}},
			wantErr: apperrors.ErrInvalidNamespace,
		},
		{
			name: "trailing slash ref component is an invalid namespace",
			ns:   selectorBare.WithRef("feat/"),
			m: &mocks.Manifold{SnapshotResult: domain.RefSnapshot{
				Branches: map[string]string{"feat/": "cfeat", "main": "cmain"},
				Head:     "main",
			}},
			wantErr: apperrors.ErrInvalidNamespace,
		},
		{
			name: "leading slash ref component is an invalid namespace",
			ns:   selectorBare.WithRef("/feat"),
			m: &mocks.Manifold{SnapshotResult: domain.RefSnapshot{
				Branches: map[string]string{"/feat": "cfeat", "main": "cmain"},
				Head:     "main",
			}},
			wantErr: apperrors.ErrInvalidNamespace,
		},
		{
			name:    "malformed glob is an invalid namespace",
			ns:      selectorBare.WithRef("v1.[*"),
			m:       &mocks.Manifold{SnapshotResult: selectorSnapshot()},
			wantErr: apperrors.ErrInvalidNamespace,
		},
		{
			name:    "glob matching nothing is an invalid namespace",
			ns:      selectorBare.WithRef("v9.*"),
			m:       &mocks.Manifold{SnapshotResult: selectorSnapshot()},
			wantErr: apperrors.ErrInvalidNamespace,
		},
		{
			name:    "refless with an empty repository is not found",
			ns:      selectorBare,
			m:       &mocks.Manifold{SnapshotResult: domain.RefSnapshot{}},
			wantErr: apperrors.ErrNotFound,
		},
		{
			name: "channel with no resolvable commit is not found",
			ns:   selectorBare.WithRef("main"),
			m: &mocks.Manifold{
				SnapshotResult: domain.RefSnapshot{Head: "main"},
			},
			wantErr: apperrors.ErrNotFound,
		},
		{
			name: "refless default channel with no resolvable commit is not found",
			ns:   selectorBare,
			m: &mocks.Manifold{
				SnapshotResult: domain.RefSnapshot{Head: "main"},
			},
			wantErr: apperrors.ErrNotFound,
		},
		{
			name:    "snapshot not found is not found",
			ns:      selectorBare.WithRef("stable"),
			m:       &mocks.Manifold{SnapshotErr: manifoldresolver.ErrNotFound},
			wantErr: apperrors.ErrNotFound,
		},
		{
			name:    "snapshot failure keeps its cause",
			ns:      selectorBare.WithRef("stable"),
			m:       &mocks.Manifold{SnapshotErr: fetchErr},
			wantErr: fetchErr,
		},
		{
			name: "manifest fetch failure is a fetch failure",
			ns:   selectorBare.WithRef("stable"),
			m: &mocks.Manifold{
				SnapshotResult:          selectorSnapshot(),
				ResolveArrowAtCommitErr: manifoldresolver.ErrFetchFailed,
			},
			wantErr: apperrors.ErrFetchFailed,
		},
		{
			name: "cache purge failure is returned",
			ns:   selectorBare.WithRef("stable"),
			m: &mocks.Manifold{
				SnapshotResult:             selectorSnapshot(),
				ResolveArrowAtCommitResult: &domain.Arrow{},
			},
			v:       &mocks.Vault{DeleteArrowErr: fetchErr},
			wantErr: fetchErr,
		},
		{
			name: "cache write failure is returned",
			ns:   selectorBare.WithRef("stable"),
			m: &mocks.Manifold{
				SnapshotResult:             selectorSnapshot(),
				ResolveArrowAtCommitResult: &domain.Arrow{},
			},
			v:       &mocks.Vault{PutArrowErr: fetchErr},
			wantErr: fetchErr,
		},
		{
			name: "a workdir another identity owns is a conflict",
			ns:   selectorBare.WithRef("stable"),
			m: &mocks.Manifold{
				SnapshotResult:             selectorSnapshot(),
				ResolveArrowAtCommitResult: &domain.Arrow{},
			},
			v:       &mocks.Vault{PutArrowErr: vault.ErrWorkDirCollision},
			wantErr: apperrors.ErrAlreadyExists,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var v vault.Vault
			if tc.v != nil {
				v = tc.v
			}
			r := newTestReaderWithVaultManifold(t, v, tc.m)

			_, arrow, err := r.ResolveInstall(context.Background(), tc.ns, store.CacheWhenAbsent(absent))
			require.ErrorIs(t, err, tc.wantErr)
			assert.Nil(t, arrow)
		})
	}
}

func refFailingManifold(
	snap domain.RefSnapshot,
	failures map[string]error,
	fetched *[]string,
) *mocks.Manifold {
	return &mocks.Manifold{
		SnapshotResult: snap,
		ResolveArrowAtCommitFn: func(
			_ context.Context,
			ns domain.Namespace,
			ref string,
			_ string,
		) (*domain.Arrow, []byte, string, error) {
			*fetched = append(*fetched, ref)
			if err, failed := failures[ref]; failed {
				return nil, nil, "", err
			}
			return &domain.Arrow{Namespace: ns}, []byte("raw"), "ARROW.md", nil
		},
	}
}

// A refless install whose latest stable release serves no manifest settles on
// the next listed channel that does, then on the HEAD branch.
func TestResolveInstall_ReflessStableWithoutManifest(t *testing.T) {
	snap := domain.RefSnapshot{
		Tags:     map[string]string{"v1.3.1": "c131", "tip": "ctip", "bad//tag": "cbad"},
		Branches: map[string]string{"main": "cmain"},
		Head:     "main",
	}
	missing := manifoldresolver.ErrNotFound

	testCases := []struct {
		name         string
		ns           domain.Namespace
		failures     map[string]error
		wantErr      error
		wantIdentity domain.Namespace
		wantKind     domain.SelectorKind
		wantFetched  []string
	}{
		{
			name:         "other listed channel",
			ns:           selectorBare,
			failures:     map[string]error{"v1.3.1": missing},
			wantIdentity: selectorBare.WithRef("tip"),
			wantKind:     domain.SelectorPointerChannel,
			wantFetched:  []string{"v1.3.1", "tip"},
		},
		{
			name:         "head branch",
			ns:           selectorBare,
			failures:     map[string]error{"v1.3.1": missing, "tip": missing},
			wantIdentity: selectorBare.WithRef("main"),
			wantKind:     domain.SelectorBranchPin,
			wantFetched:  []string{"v1.3.1", "tip", "main"},
		},
		{
			name: "every fallback failing keeps the stable not-found",
			ns:   selectorBare,
			failures: map[string]error{
				"v1.3.1": missing,
				"tip":    missing,
				"main":   manifoldresolver.ErrFetchFailed,
			},
			wantErr:     apperrors.ErrNotFound,
			wantFetched: []string{"v1.3.1", "tip", "main"},
		},
		{
			name:        "fetch failure does not fall back",
			ns:          selectorBare,
			failures:    map[string]error{"v1.3.1": manifoldresolver.ErrFetchFailed},
			wantErr:     apperrors.ErrFetchFailed,
			wantFetched: []string{"v1.3.1"},
		},
		{
			name:        "an explicit selector does not fall back",
			ns:          selectorBare.WithRef("stable"),
			failures:    map[string]error{"v1.3.1": missing},
			wantErr:     apperrors.ErrNotFound,
			wantFetched: []string{"v1.3.1"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var fetched []string
			r := newTestReaderWithVaultManifold(t, nil, refFailingManifold(snap, tc.failures, &fetched))

			identity, arrow, err := r.ResolveInstall(context.Background(), tc.ns)

			assert.Equal(t, tc.wantFetched, fetched)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				assert.Nil(t, arrow)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantIdentity, identity)
			assert.Equal(t, tc.wantKind, arrow.SelectorKind)
		})
	}
}

func TestCheckDrift(t *testing.T) {
	moved := selectorSnapshot()
	moved.Tags["nightly"] = "cnightly2"
	moved.Tags["v1.2.0"] = "c120b"
	moved.Tags["v1.4.0"] = "c140"

	testCases := []struct {
		name          string
		arrow         domain.Arrow
		snap          domain.RefSnapshot
		snapErr       error
		wantAvailable *domain.Available
		wantOK        bool
	}{
		{
			name: "rolling tag moved is outdated on the same tag",
			arrow: domain.Arrow{
				Namespace:    selectorBare.WithRef("nightly"),
				SelectorKind: domain.SelectorChannel,
				Resolved:     domain.Resolved{Ref: "nightly", Commit: "cnightly"},
			},
			snap:          moved,
			wantAvailable: &domain.Available{Ref: "nightly", Commit: "cnightly2"},
			wantOK:        true,
		},
		{
			name: "rolling tag in place is current",
			arrow: domain.Arrow{
				Namespace:    selectorBare.WithRef("nightly"),
				SelectorKind: domain.SelectorChannel,
				Resolved:     domain.Resolved{Ref: "nightly", Commit: "cnightly"},
			},
			snap:   selectorSnapshot(),
			wantOK: true,
		},
		{
			name: "stable channel with a newer release is outdated",
			arrow: domain.Arrow{
				Namespace:    selectorBare.WithRef("stable"),
				SelectorKind: domain.SelectorChannel,
				Resolved:     domain.Resolved{Ref: "v1.3.0", Commit: "c130"},
			},
			snap:          selectorSnapshot(),
			wantAvailable: &domain.Available{Ref: "v2.0.0", Commit: "c200"},
			wantOK:        true,
		},
		{
			name: "ordered tag force-moved is outdated on the same pin",
			arrow: domain.Arrow{
				Namespace:    selectorBare.WithRef("v1.2.0"),
				SelectorKind: domain.SelectorPin,
				Resolved:     domain.Resolved{Ref: "v1.2.0", Commit: "c120"},
			},
			snap:          moved,
			wantAvailable: &domain.Available{Ref: "v1.2.0", Commit: "c120b"},
			wantOK:        true,
		},
		{
			name: "pin never follows a newer release",
			arrow: domain.Arrow{
				Namespace:    selectorBare.WithRef("v1.2.0"),
				SelectorKind: domain.SelectorPin,
				Resolved:     domain.Resolved{Ref: "v1.2.0", Commit: "c120"},
			},
			snap:   selectorSnapshot(),
			wantOK: true,
		},
		{
			name: "constraint with a newer matching tag is outdated within bounds",
			arrow: domain.Arrow{
				Namespace:    selectorBare.WithRef("v1.*"),
				SelectorKind: domain.SelectorConstraint,
				Resolved:     domain.Resolved{Ref: "v1.3.0", Commit: "c130"},
			},
			snap:          moved,
			wantAvailable: &domain.Available{Ref: "v1.4.0", Commit: "c140"},
			wantOK:        true,
		},
		{
			name: "commit pin is never outdated",
			arrow: domain.Arrow{
				Namespace:    selectorBare.WithRef(headSHA),
				SelectorKind: domain.SelectorCommit,
				Resolved:     domain.Resolved{Ref: headSHA, Commit: headSHA},
			},
			snap:   moved,
			wantOK: true,
		},
		{
			name: "escaped pin on its short ref is current",
			arrow: domain.Arrow{
				Namespace:    selectorBare.WithRef("refs/tags/v1.0.0"),
				SelectorKind: domain.SelectorPin,
				Resolved:     domain.Resolved{Ref: "v1.0.0", Commit: "c100"},
			},
			snap:   moved,
			wantOK: true,
		},
		{
			name:          "legacy row with no kind and no resolved state is outdated once",
			arrow:         domain.Arrow{Namespace: selectorBare.WithRef("nightly")},
			snap:          moved,
			wantAvailable: &domain.Available{Ref: "nightly", Commit: "cnightly2"},
			wantOK:        true,
		},
		{
			name: "legacy row once resolved is current",
			arrow: domain.Arrow{
				Namespace: selectorBare.WithRef("nightly"),
				Resolved:  domain.Resolved{Ref: "nightly", Commit: "cnightly2"},
			},
			snap:   moved,
			wantOK: true,
		},
		{
			name: "vanished target writes no answer",
			arrow: domain.Arrow{
				Namespace:    selectorBare.WithRef("v9.9.9"),
				SelectorKind: domain.SelectorPin,
				Resolved:     domain.Resolved{Ref: "v9.9.9", Commit: "c999"},
			},
			snap: selectorSnapshot(),
		},
		{
			name: "snapshot failure writes no answer",
			arrow: domain.Arrow{
				Namespace:    selectorBare.WithRef("stable"),
				SelectorKind: domain.SelectorChannel,
				Resolved:     domain.Resolved{Ref: "v1.3.0", Commit: "c130"},
			},
			snapErr: manifoldresolver.ErrFetchFailed,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m := &mocks.Manifold{SnapshotResult: tc.snap, SnapshotErr: tc.snapErr}
			r := newTestReaderWithVaultManifold(t, nil, m)

			available, ok := r.CheckDrift(context.Background(), tc.arrow)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.wantAvailable, available)
		})
	}
}

func TestProject_CarriesSelectorStateIntoTheReadModel(t *testing.T) {
	r := newTestReader(t)
	arrow := domain.Arrow{
		Namespace:    selectorBare.WithRef("stable"),
		SelectorKind: domain.SelectorChannel,
		Resolved:     domain.Resolved{Ref: "v2.0.0", Commit: "c200", Fingerprint: "c200"},
		Available:    &domain.Available{Ref: "v2.1.0", Commit: "c210"},
	}
	seedArrow(t, r, arrow)

	got, err := r.Get(context.Background(), arrow.Namespace)
	require.NoError(t, err)
	assert.Equal(t, domain.SelectorChannel, got.SelectorKind)
	assert.Equal(t, arrow.Resolved, got.Resolved)
	assert.Equal(t, arrow.Available, got.Available)
}

// A dependency identity like pkg@v1.* names no ref a host can serve, so a
// manifest for one that is not cached yet is read at its selector's target
// commit instead of failing the whole dependency walk.
func TestResolveManifest_SelectorIdentityFallsBackToItsTarget(t *testing.T) {
	fetchErr := manifoldresolver.ErrNotFound

	testCases := []struct {
		name        string
		ns          domain.Namespace
		snapErr     error
		wantErr     error
		wantFetched []commitFetch
	}{
		{
			name:        "constraint reads its highest match",
			ns:          selectorBare.WithRef("v1.*"),
			wantFetched: []commitFetch{{ns: selectorBare.WithRef("v1.*"), ref: "v1.3.0", commit: "c130"}},
		},
		{
			name:        "channel reads its latest member",
			ns:          selectorBare.WithRef("stable"),
			wantFetched: []commitFetch{{ns: selectorBare.WithRef("stable"), ref: "v2.0.0", commit: "c200"}},
		},
		{
			name:    "an unresolvable selector reports the original failure",
			ns:      selectorBare.WithRef("v9.*"),
			wantErr: apperrors.ErrNotFound,
		},
		{
			name:    "a failed snapshot reports the original failure",
			ns:      selectorBare.WithRef("v1.*"),
			snapErr: errors.New("remote down"),
			wantErr: apperrors.ErrNotFound,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var fetched []commitFetch
			m := selectorManifold(selectorSnapshot(), &fetched)
			m.SnapshotErr = tc.snapErr
			m.ResolveArrowErr = fetchErr

			arrow, err := newTestReaderWithVaultManifold(t, nil, m).ResolveManifest(context.Background(), tc.ns)

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				assert.Empty(t, fetched)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.ns, arrow.Namespace)
			assert.Equal(t, "crowbar", arrow.Name)
			assert.Equal(t, tc.wantFetched, fetched)
		})
	}
}

// A passive check already runs at most once per version-check TTL, so it
// reads the remote live: the shared snapshot cache is refilled by listings
// and installs at arbitrary times and could hide a moved tag for another TTL.
func TestCheckDrift_ReadsTheRemoteLive(t *testing.T) {
	cached := domain.RefSnapshot{Tags: map[string]string{"nightly": "cold"}}
	live := domain.RefSnapshot{Tags: map[string]string{"nightly": "cnew"}}
	m := &mocks.Manifold{
		SnapshotResult: cached,
		FreshSnapshotFn: func(context.Context, domain.Namespace) (domain.RefSnapshot, error) {
			return live, nil
		},
	}
	r := newTestReaderWithVaultManifold(t, nil, m)
	row := domain.Arrow{
		Namespace:    selectorBare.WithRef("nightly"),
		SelectorKind: domain.SelectorChannel,
		Resolved:     domain.Resolved{Ref: "nightly", Commit: "cold"},
	}

	available, ok := r.CheckDrift(context.Background(), row)

	require.True(t, ok)
	assert.Equal(t, &domain.Available{Ref: "nightly", Commit: "cnew"}, available)
	assert.Zero(t, m.SnapshotCalls)
}

// A refless preview files what it had to fetch, so the next one reuses it and
// fetches nothing, however long ago the first was.
func TestResolveManifest_Refless_SecondPreviewReusesTheFiledManifest(t *testing.T) {
	v := realVault(t)
	fetches := 0
	r := newTestReaderWithVaultManifold(t, v, countingFetches(selectorSnapshot(), nil, &fetches))

	first, err := r.ResolveManifest(context.Background(), selectorBare)
	require.NoError(t, err)
	require.Equal(t, 1, fetches)

	second, err := r.ResolveManifest(context.Background(), selectorBare)
	store.WaitRechecks(r)

	require.NoError(t, err)
	assert.Equal(t, 1, fetches, "the filed manifest is served, not fetched again")
	assert.Equal(t, first.Namespace, second.Namespace)
	assert.Equal(t, first.Resolved, second.Resolved)
}

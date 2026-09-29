package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
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
			commit string,
		) (*domain.Arrow, []byte, string, error) {
			*fetched = append(*fetched, commitFetch{ns: ns, commit: commit})
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
			wantKind:     domain.SelectorChannel,
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
			wantKind:     domain.SelectorChannel,
			wantResolved: domain.Resolved{Ref: "main", Commit: "cmain", Fingerprint: "cmain"},
		},
		{
			name:         "stable channel keeps its identity",
			ns:           selectorBare.WithRef("stable"),
			snap:         selectorSnapshot(),
			wantIdentity: selectorBare.WithRef("stable"),
			wantKind:     domain.SelectorChannel,
			wantResolved: domain.Resolved{Ref: "v2.0.0", Commit: "c200", Fingerprint: "c200"},
		},
		{
			name:         "rolling tag is a channel at the tag's commit",
			ns:           selectorBare.WithRef("nightly"),
			snap:         selectorSnapshot(),
			wantIdentity: selectorBare.WithRef("nightly"),
			wantKind:     domain.SelectorChannel,
			wantResolved: domain.Resolved{Ref: "nightly", Commit: "cnightly", Fingerprint: "cnightly"},
		},
		{
			name:         "exact tag is a pin",
			ns:           selectorBare.WithRef("v1.2.0"),
			snap:         selectorSnapshot(),
			wantIdentity: selectorBare.WithRef("v1.2.0"),
			wantKind:     domain.SelectorPin,
			wantResolved: domain.Resolved{Ref: "v1.2.0", Commit: "c120", Fingerprint: "c120"},
		},
		{
			name:         "branch is a pin",
			ns:           selectorBare.WithRef("main"),
			snap:         selectorSnapshot(),
			wantIdentity: selectorBare.WithRef("main"),
			wantKind:     domain.SelectorPin,
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
			name: "escaped tag named like a channel is a pin on the short ref",
			ns:   selectorBare.WithRef("refs/tags/stable"),
			snap: domain.RefSnapshot{
				Tags: map[string]string{"stable": "cst", "v1.0.0": "c100"},
			},
			wantIdentity: selectorBare.WithRef("refs/tags/stable"),
			wantKind:     domain.SelectorPin,
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
			assert.Empty(t, arrow.InstalledConstraint)
			assert.Equal(t, []commitFetch{{ns: tc.wantIdentity, commit: tc.wantResolved.Commit}}, fetched)
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

func TestResolveInstall_CachesTheManifestUnderTheIdentity(t *testing.T) {
	var fetched []commitFetch
	v := &mocks.Vault{}
	r := newTestReaderWithVaultManifold(t, v, selectorManifold(selectorSnapshot(), &fetched))

	identity, _, err := r.ResolveInstall(context.Background(), selectorBare)
	require.NoError(t, err)

	assert.Equal(t, []string{"delete " + identity.String(), "put " + identity.String()}, v.ArrowOps)
	require.Len(t, v.PutArrowFiles, 1)
	assert.Equal(t, []byte("raw"), v.PutArrowFiles[0].Content)
	assert.Equal(t, "ARROW.md", v.PutArrowFiles[0].Filename)
	assert.NotNil(t, v.PutArrowFiles[0].Meta)
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
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var v vault.Vault
			if tc.v != nil {
				v = tc.v
			}
			r := newTestReaderWithVaultManifold(t, v, tc.m)

			_, arrow, err := r.ResolveInstall(context.Background(), tc.ns)
			require.ErrorIs(t, err, tc.wantErr)
			assert.Nil(t, arrow)
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

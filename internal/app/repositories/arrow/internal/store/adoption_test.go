package store_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	manifoldresolver "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver"
	"github.com/rabbytesoftware/quiver.core/internal/mocks"
)

func TestResolveAdoption_Selectors(t *testing.T) {
	testCases := []struct {
		name         string
		ns           domain.Namespace
		ref          string
		wantIdentity domain.Namespace
		wantKind     domain.SelectorKind
		wantResolved domain.Resolved
	}{
		{
			name:         "channel adopts a member older than its newest",
			ns:           selectorBare.WithRef("stable"),
			ref:          "v1.2.0",
			wantIdentity: selectorBare.WithRef("stable"),
			wantKind:     domain.SelectorChannel,
			wantResolved: domain.Resolved{Ref: "v1.2.0", Commit: "c120", Fingerprint: "c120"},
		},
		{
			name:         "refless adopts under the default channel",
			ns:           selectorBare,
			ref:          "v1.0.0",
			wantIdentity: selectorBare.WithRef("stable"),
			wantKind:     domain.SelectorChannel,
			wantResolved: domain.Resolved{Ref: "v1.0.0", Commit: "c100", Fingerprint: "c100"},
		},
		{
			name:         "pointer channel adopts its own tag",
			ns:           selectorBare.WithRef("nightly"),
			ref:          "nightly",
			wantIdentity: selectorBare.WithRef("nightly"),
			wantKind:     domain.SelectorChannel,
			wantResolved: domain.Resolved{Ref: "nightly", Commit: "cnightly", Fingerprint: "cnightly"},
		},
		{
			name:         "constraint adopts a matching tag below its highest",
			ns:           selectorBare.WithRef("v1.*"),
			ref:          "v1.0.0",
			wantIdentity: selectorBare.WithRef("v1.*"),
			wantKind:     domain.SelectorConstraint,
			wantResolved: domain.Resolved{Ref: "v1.0.0", Commit: "c100", Fingerprint: "c100"},
		},
		{
			name:         "pin adopts its own ref",
			ns:           selectorBare.WithRef("v1.2.0"),
			ref:          "v1.2.0",
			wantIdentity: selectorBare.WithRef("v1.2.0"),
			wantKind:     domain.SelectorPin,
			wantResolved: domain.Resolved{Ref: "v1.2.0", Commit: "c120", Fingerprint: "c120"},
		},
		{
			name:         "commit selector adopts itself",
			ns:           selectorBare.WithRef(headSHA),
			ref:          headSHA,
			wantIdentity: selectorBare.WithRef(headSHA),
			wantKind:     domain.SelectorCommit,
			wantResolved: domain.Resolved{Ref: headSHA, Commit: headSHA, Fingerprint: headSHA},
		},
		{
			name:         "commit selector in upper case adopts under the lower-case identity",
			ns:           selectorBare.WithRef(strings.ToUpper(headSHA)),
			ref:          strings.ToUpper(headSHA),
			wantIdentity: selectorBare.WithRef(headSHA),
			wantKind:     domain.SelectorCommit,
			wantResolved: domain.Resolved{Ref: headSHA, Commit: headSHA, Fingerprint: headSHA},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var fetched []commitFetch
			v := &mocks.Vault{}
			m := selectorManifold(selectorSnapshot(), &fetched)
			r := newTestReaderWithVaultManifold(t, v, m)

			got, err := r.ResolveAdoption(context.Background(), tc.ns, tc.ref)
			require.NoError(t, err)

			assert.Equal(t, tc.wantIdentity, got.Identity)
			assert.Equal(t, tc.wantKind, got.Kind)
			assert.Equal(t, tc.wantResolved, got.Resolved)
			assert.Equal(t, []byte("raw"), got.Manifest)
			assert.Equal(t, "ARROW.md", got.Filename)
			assert.Equal(t, []commitFetch{{ns: tc.wantIdentity, ref: tc.wantResolved.Ref, commit: tc.wantResolved.Commit}}, fetched)
			assert.Equal(t, 1, m.FreshSnapshotCalls)
			assert.Zero(t, m.SnapshotCalls)
			assert.Empty(t, v.ArrowOps)
		})
	}
}

// An adopted older member is what the row has installed, so the next check
// reports the channel's newest as available.
func TestResolveAdoption_OlderMember_DriftsToTheNewest(t *testing.T) {
	var fetched []commitFetch
	r := newTestReaderWithVaultManifold(t, nil, selectorManifold(selectorSnapshot(), &fetched))

	got, err := r.ResolveAdoption(context.Background(), selectorBare.WithRef("stable"), "v1.2.0")
	require.NoError(t, err)

	available, ok := r.CheckDrift(context.Background(), domain.Arrow{
		Namespace:    got.Identity,
		SelectorKind: got.Kind,
		Resolved:     got.Resolved,
	})
	require.True(t, ok)
	assert.Equal(t, &domain.Available{Ref: "v2.0.0", Commit: "c200"}, available)
}

func TestResolveAdoption_Errors(t *testing.T) {
	fetchErr := errors.New("boom")

	testCases := []struct {
		name          string
		ns            domain.Namespace
		ref           string
		m             *mocks.Manifold
		wantErr       error
		wantNoNetwork bool
	}{
		{
			name:          "empty ref is an invalid namespace",
			ns:            selectorBare.WithRef("stable"),
			ref:           "",
			m:             &mocks.Manifold{SnapshotResult: selectorSnapshot()},
			wantErr:       apperrors.ErrInvalidNamespace,
			wantNoNetwork: true,
		},
		{
			name:          "blank ref is an invalid namespace",
			ns:            selectorBare.WithRef("stable"),
			ref:           "  ",
			m:             &mocks.Manifold{SnapshotResult: selectorSnapshot()},
			wantErr:       apperrors.ErrInvalidNamespace,
			wantNoNetwork: true,
		},
		{
			name:    "ref the remote does not hold is not found",
			ns:      selectorBare.WithRef("stable"),
			ref:     "v0.0.1",
			m:       &mocks.Manifold{SnapshotResult: selectorSnapshot()},
			wantErr: apperrors.ErrNotFound,
		},
		{
			name:    "ref outside the channel is an invalid namespace",
			ns:      selectorBare.WithRef("stable"),
			ref:     "nightly",
			m:       &mocks.Manifold{SnapshotResult: selectorSnapshot()},
			wantErr: apperrors.ErrInvalidNamespace,
		},
		{
			name:    "pointer channel refuses another tag",
			ns:      selectorBare.WithRef("nightly"),
			ref:     "v2.0.0",
			m:       &mocks.Manifold{SnapshotResult: selectorSnapshot()},
			wantErr: apperrors.ErrInvalidNamespace,
		},
		{
			name:    "constraint refuses a tag outside its glob",
			ns:      selectorBare.WithRef("v1.*"),
			ref:     "v2.0.0",
			m:       &mocks.Manifold{SnapshotResult: selectorSnapshot()},
			wantErr: apperrors.ErrInvalidNamespace,
		},
		{
			name:    "pin refuses a different ref",
			ns:      selectorBare.WithRef("v1.2.0"),
			ref:     "v1.3.0",
			m:       &mocks.Manifold{SnapshotResult: selectorSnapshot()},
			wantErr: apperrors.ErrInvalidNamespace,
		},
		{
			name:    "commit selector refuses a ref at another commit",
			ns:      selectorBare.WithRef(headSHA),
			ref:     "v1.2.0",
			m:       &mocks.Manifold{SnapshotResult: selectorSnapshot()},
			wantErr: apperrors.ErrInvalidNamespace,
		},
		{
			name:    "unknown selector is an invalid namespace",
			ns:      selectorBare.WithRef("no-such-ref"),
			ref:     "v1.2.0",
			m:       &mocks.Manifold{SnapshotResult: selectorSnapshot()},
			wantErr: apperrors.ErrInvalidNamespace,
		},
		{
			name:    "refless with an empty repository is not found",
			ns:      selectorBare,
			ref:     "v1.2.0",
			m:       &mocks.Manifold{SnapshotResult: domain.RefSnapshot{}},
			wantErr: apperrors.ErrNotFound,
		},
		{
			name:    "snapshot not found is not found",
			ns:      selectorBare.WithRef("stable"),
			ref:     "v1.2.0",
			m:       &mocks.Manifold{SnapshotErr: manifoldresolver.ErrNotFound},
			wantErr: apperrors.ErrNotFound,
		},
		{
			name:    "snapshot failure keeps its cause",
			ns:      selectorBare.WithRef("stable"),
			ref:     "v1.2.0",
			m:       &mocks.Manifold{SnapshotErr: fetchErr},
			wantErr: fetchErr,
		},
		{
			name: "manifest fetch failure is a fetch failure",
			ns:   selectorBare.WithRef("stable"),
			ref:  "v1.2.0",
			m: &mocks.Manifold{
				SnapshotResult:          selectorSnapshot(),
				ResolveArrowAtCommitErr: manifoldresolver.ErrFetchFailed,
			},
			wantErr: apperrors.ErrFetchFailed,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := newTestReaderWithVaultManifold(t, nil, tc.m)

			_, err := r.ResolveAdoption(context.Background(), tc.ns, tc.ref)
			require.ErrorIs(t, err, tc.wantErr)
			if tc.wantNoNetwork {
				assert.Zero(t, tc.m.FreshSnapshotCalls)
			}
		})
	}
}

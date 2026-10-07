package store_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	manifoldresolver "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
	"github.com/rabbytesoftware/quiver.core/internal/mocks"
)

// headManifold serves a channel whose head moved past what the row installed.
func headManifold() *mocks.Manifold {
	head := func() *domain.Arrow { return &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "Head"}} }
	return &mocks.Manifold{
		SnapshotResult:   domain.RefSnapshot{Tags: map[string]string{"v1.2.0": "c1", "v1.3.0": "c2"}},
		ParseArrowResult: head(),
		ResolveArrowFunc: func(context.Context, domain.Namespace) (*domain.Arrow, []byte, string, error) {
			return head(), []byte("head"), "ARROW.md", nil
		},
		ResolveArrowAtCommitFn: func(context.Context, domain.Namespace, string, string) (*domain.Arrow, []byte, string, error) {
			return head(), []byte("head"), "ARROW.md", nil
		},
	}
}

// A catalogued identity reads the manifest its row installed, whatever the
// vault cache holds for it and wherever its selector points now.
func TestResolveManifest_CataloguedIdentity_ServesTheRow(t *testing.T) {
	testCases := []struct {
		name  string
		ns    domain.Namespace
		vault *mocks.Vault
	}{
		{name: "expired cache", ns: "github.com/user/tool@stable", vault: &mocks.Vault{GetArrowErr: vault.ErrStale, GetArrowFile: vault.ManifestFile{Content: []byte("old")}}},
		{name: "confirmed absent marker", ns: "github.com/user/tool@stable", vault: &mocks.Vault{GetArrowErr: vault.ErrConfirmedAbsent}},
		{name: "not cached", ns: "github.com/user/tool@stable", vault: &mocks.Vault{GetArrowErr: vault.ErrNotCached}},
		{name: "bare namespace", ns: "github.com/user/tool", vault: &mocks.Vault{GetArrowErr: vault.ErrStale, GetArrowFile: vault.ManifestFile{Content: []byte("old")}}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			identity := domain.Namespace("github.com/user/tool@stable")
			r := newTestReaderWithVaultManifold(t, tc.vault, headManifold())
			seedArrow(t, r, domain.Arrow{
				Namespace:    identity,
				ArrowMeta:    domain.ArrowMeta{Name: "Installed"},
				Readme:       "installed readme",
				SelectorKind: domain.SelectorChannel,
				Resolved:     domain.Resolved{Ref: "v1.2.0", Commit: "c1", Fingerprint: "c1"},
			})

			got, err := r.ResolveManifest(context.Background(), tc.ns)

			require.NoError(t, err)
			assert.Equal(t, "Installed", got.Name)
			assert.Equal(t, "installed readme", got.Readme)
			assert.Equal(t, identity, got.Namespace)
			assert.Zero(t, tc.vault.PutArrowCalls, "the installed manifest's cache is never overwritten by a read")
		})
	}
}

// An identity nothing has catalogued is still previewed through the vault.
func TestResolveManifest_UncataloguedIdentity_PreviewsThroughTheVault(t *testing.T) {
	inner := &mocks.Vault{GetArrowErr: vault.ErrStale, GetArrowFile: vault.ManifestFile{Content: []byte("old")}}
	v := newSignalVault(inner)
	r := newTestReaderWithVaultManifold(t, v, headManifold())
	seedArrow(t, r, domain.Arrow{Namespace: "github.com/user/tool@v1.*", ArrowMeta: domain.ArrowMeta{Name: "Other row"}})

	got, err := r.ResolveManifest(context.Background(), "github.com/user/tool@stable")

	require.NoError(t, err)
	assert.Equal(t, "Head", got.Name)
	v.awaitPut(t)
	assert.Equal(t, 1, inner.PutArrowCalls)
}

// A bare namespace nothing has catalogued fails when its repository cannot
// be listed.
func TestResolveManifest_BareUncatalogued_SnapshotFails_ReturnsFetchFailed(t *testing.T) {
	m := headManifold()
	m.SnapshotErr = manifoldresolver.ErrFetchFailed
	r := newTestReaderWithVaultManifold(t, &mocks.Vault{GetArrowErr: vault.ErrNotCached}, m)

	_, err := r.ResolveManifest(context.Background(), "github.com/user/tool")

	require.ErrorIs(t, err, apperrors.ErrFetchFailed)
}

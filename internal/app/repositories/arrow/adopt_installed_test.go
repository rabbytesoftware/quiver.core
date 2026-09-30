package arrow_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	adapterSQLite "github.com/rabbytesoftware/quiver.core/internal/adapter/store/sqlite"
	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	arrowRepo "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow"
	arrowStoreMocks "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/mocks"
	arrowstore "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/store"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/mocks"
)

const adoptBare = domain.Namespace("github.com/rabbytesoftware/quiver.desktop")

func adoptSnapshot() domain.RefSnapshot {
	return domain.RefSnapshot{
		Tags: map[string]string{
			"v1.2.0":  "c120",
			"v1.3.0":  "c130",
			"nightly": "cnightly",
		},
		Branches: map[string]string{"main": "cmain"},
		Head:     "main",
	}
}

// adoptManifold serves adoptSnapshot and a manifest per commit, recording
// every commit fetched.
func adoptManifold(fetched *[]string) *mocks.Manifold {
	return &mocks.Manifold{
		SnapshotResult:   adoptSnapshot(),
		ParseArrowResult: &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "Quiver Desktop"}},
		ResolveArrowAtCommitFn: func(_ context.Context, ns domain.Namespace, _, commit string) (*domain.Arrow, []byte, string, error) {
			*fetched = append(*fetched, commit)
			return &domain.Arrow{Namespace: ns, ArrowMeta: domain.ArrowMeta{Name: "Quiver Desktop"}}, []byte("manifest"), "ARROW.md", nil
		},
	}
}

type adoptCatalog struct {
	cat     arrowRepo.Arrow
	vault   *mocks.Vault
	fetched *[]string
}

func newAdoptCatalog(t *testing.T) adoptCatalog {
	t.Helper()
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	axArrow := newTestAsynxArrow(t)
	t.Cleanup(func() { _ = axArrow.Shutdown(context.Background()) })
	v := &mocks.Vault{}
	var fetched []string
	cat, err := arrowRepo.New(db, axArrow, v, adoptManifold(&fetched), nil)
	require.NoError(t, err)
	return adoptCatalog{cat: cat, vault: v, fetched: &fetched}
}

func (a adoptCatalog) row(t *testing.T, ns domain.Namespace) *domain.Arrow {
	t.Helper()
	got, err := a.cat.Get(context.Background(), ns)
	require.NoError(t, err)
	require.NotNil(t, got)
	return got
}

func TestAdoptInstalled_OlderChannelMember_IsInstalledAndBehind(t *testing.T) {
	a := newAdoptCatalog(t)
	identity := adoptBare.WithRef("stable")

	require.NoError(t, a.cat.AdoptInstalled(context.Background(), identity, "v1.2.0"))

	got := a.row(t, identity)
	assert.Equal(t, domain.SelectorChannel, got.SelectorKind)
	assert.Equal(t, domain.Resolved{Ref: "v1.2.0", Commit: "c120", Fingerprint: "c120"}, got.Resolved)
	assert.True(t, got.UserInstalled)
	assert.Nil(t, got.Available)
	assert.Equal(t, []string{"c120"}, *a.fetched)

	userInstalled := true
	views, err := a.cat.List(context.Background(), &userInstalled)
	require.NoError(t, err)
	assert.Equal(t, []domain.Namespace{identity}, listedNamespaces(views))

	available, err := a.cat.CheckAvailable(context.Background(), identity)
	require.NoError(t, err)
	assert.Equal(t, &domain.Available{Ref: "v1.3.0", Commit: "c130"}, available)
}

func TestAdoptInstalled_Refless_AdoptsUnderTheDefaultChannel(t *testing.T) {
	a := newAdoptCatalog(t)

	require.NoError(t, a.cat.AdoptInstalled(context.Background(), adoptBare, "v1.2.0"))

	got := a.row(t, adoptBare.WithRef("stable"))
	assert.Equal(t, domain.SelectorChannel, got.SelectorKind)
	assert.Equal(t, "v1.2.0", got.Resolved.Ref)
	exists, err := a.cat.Exists(context.Background(), adoptBare)
	require.NoError(t, err)
	assert.False(t, exists)
}

// A client re-announcing the same build on every start must write nothing.
func TestAdoptInstalled_SameState_IsIdempotent(t *testing.T) {
	a := newAdoptCatalog(t)
	identity := adoptBare.WithRef("stable")
	require.NoError(t, a.cat.AdoptInstalled(context.Background(), identity, "v1.2.0"))
	before := a.row(t, identity)
	opsBefore := len(a.vault.ArrowOps)

	require.NoError(t, a.cat.AdoptInstalled(context.Background(), identity, "v1.2.0"))

	after := a.row(t, identity)
	assert.Equal(t, before.Resolved, after.Resolved)
	assert.True(t, after.UserInstalled)
	assert.Len(t, a.vault.ArrowOps, opsBefore)
}

func TestAdoptInstalled_NewerState_AdvancesInPlace(t *testing.T) {
	a := newAdoptCatalog(t)
	identity := adoptBare.WithRef("stable")
	require.NoError(t, a.cat.AdoptInstalled(context.Background(), identity, "v1.2.0"))

	require.NoError(t, a.cat.AdoptInstalled(context.Background(), identity, "v1.3.0"))

	got := a.row(t, identity)
	assert.Equal(t, identity, got.Namespace)
	assert.Equal(t, domain.SelectorChannel, got.SelectorKind)
	assert.Equal(t, domain.Resolved{Ref: "v1.3.0", Commit: "c130", Fingerprint: "c130"}, got.Resolved)
	assert.True(t, got.UserInstalled)

	available, err := a.cat.CheckAvailable(context.Background(), identity)
	require.NoError(t, err)
	assert.Nil(t, available)
}

// A row Add filed at the newest member moves back to the member the caller
// actually runs, so the update it needs is offered again.
func TestAdoptInstalled_RowAddedAtNewest_MovesToTheDeclaredState(t *testing.T) {
	a := newAdoptCatalog(t)
	identity := adoptBare.WithRef("stable")
	require.NoError(t, a.cat.Add(context.Background(), identity))
	require.Equal(t, "v1.3.0", a.row(t, identity).Resolved.Ref)

	require.NoError(t, a.cat.AdoptInstalled(context.Background(), identity, "v1.2.0"))

	got := a.row(t, identity)
	assert.Equal(t, identity, got.Namespace)
	assert.Equal(t, domain.Resolved{Ref: "v1.2.0", Commit: "c120", Fingerprint: "c120"}, got.Resolved)
	assert.True(t, got.UserInstalled)

	available, err := a.cat.CheckAvailable(context.Background(), identity)
	require.NoError(t, err)
	assert.Equal(t, &domain.Available{Ref: "v1.3.0", Commit: "c130"}, available)
}

func TestAdoptInstalled_Refusals_WriteNothing(t *testing.T) {
	testCases := []struct {
		name    string
		ns      domain.Namespace
		ref     string
		wantErr error
	}{
		{name: "ref outside the channel", ns: adoptBare.WithRef("stable"), ref: "nightly", wantErr: apperrors.ErrInvalidNamespace},
		{name: "ref the remote does not hold", ns: adoptBare.WithRef("stable"), ref: "v9.9.9", wantErr: apperrors.ErrNotFound},
		{name: "empty ref", ns: adoptBare.WithRef("stable"), ref: "", wantErr: apperrors.ErrInvalidNamespace},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			a := newAdoptCatalog(t)

			err := a.cat.AdoptInstalled(context.Background(), tc.ns, tc.ref)

			require.ErrorIs(t, err, tc.wantErr)
			exists, existsErr := a.cat.Exists(context.Background(), tc.ns)
			require.NoError(t, existsErr)
			assert.False(t, exists)
			assert.Empty(t, a.vault.ArrowOps)
		})
	}
}

// A remote that cannot be read is a gateway failure, the same as for Add.
func TestAdoptInstalled_UnclassifiedResolveFailure_IsAFetchFailure(t *testing.T) {
	r := &arrowStoreMocks.MockCQRS{
		ResolveAdoptionFn: func(context.Context, domain.Namespace, string) (arrowstore.Adoption, error) {
			return arrowstore.Adoption{}, errors.New("connection reset")
		},
	}
	cat := arrowRepo.NewTestable(r, newTestAsynxArrow(t), &mocks.Vault{}, &mocks.Manifold{})

	err := cat.AdoptInstalled(context.Background(), adoptBare.WithRef("stable"), "v1.2.0")

	require.ErrorIs(t, err, apperrors.ErrFetchFailed)
}

func TestAdoptInstalled_ManifestThatDoesNotParse_IsAnInvalidManifest(t *testing.T) {
	r := &arrowStoreMocks.MockCQRS{
		ResolveAdoptionFn: func(_ context.Context, ns domain.Namespace, ref string) (arrowstore.Adoption, error) {
			return arrowstore.Adoption{
				Identity: ns,
				Kind:     domain.SelectorChannel,
				Resolved: domain.Resolved{Ref: ref, Commit: "c120"},
				Manifest: []byte("not a manifest"),
				Filename: "ARROW.md",
			}, nil
		},
	}
	m := &mocks.Manifold{ParseArrowErr: errors.New("bad yaml")}
	cat := arrowRepo.NewTestable(r, newTestAsynxArrow(t), &mocks.Vault{}, m)

	err := cat.AdoptInstalled(context.Background(), adoptBare.WithRef("stable"), "v1.2.0")

	require.ErrorIs(t, err, apperrors.ErrInvalidManifest)
}

// Two spellings of one commit selector are one row, so they can never share
// (and delete) each other's workdir.
func TestAdd_CommitSelectorInTwoCases_IsOneRow(t *testing.T) {
	a := newAdoptCatalog(t)
	lower := adoptBare.WithRef("abcdef1")

	require.NoError(t, a.cat.Add(context.Background(), adoptBare.WithRef("ABCDEF1")))
	require.NoError(t, a.cat.Add(context.Background(), lower))

	exists, err := a.cat.Exists(context.Background(), adoptBare.WithRef("ABCDEF1"))
	require.NoError(t, err)
	assert.False(t, exists)
	assert.Equal(t, lower, a.row(t, lower).Namespace)
	identity, err := a.cat.ResolveCatalogued(context.Background(), adoptBare.WithRef("AbCdEf1"))
	require.NoError(t, err)
	assert.Equal(t, lower, identity)
}

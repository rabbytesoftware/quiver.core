package store_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// The catalog holds arrows under a ref, but every command accepts a bare
// namespace. Resolving it is what stops `install <bare>` rejecting the
// namespace `arrow add <bare>` has just accepted.
func TestResolveCatalogued_BareNamespaceResolvesToTheStoredRef(t *testing.T) {
	r := newTestReader(t)
	versioned := domain.Namespace("github.com/user/pkg@v1.0.0")
	seedArrow(t, r, domain.Arrow{Namespace: versioned, ArrowMeta: domain.ArrowMeta{Name: "Pkg"}})

	got, err := r.ResolveCatalogued(context.Background(), "github.com/user/pkg")

	require.NoError(t, err)
	assert.Equal(t, versioned, got)
}

func TestResolveCatalogued_ExplicitRefIsHonoured(t *testing.T) {
	r := newTestReader(t)
	versioned := domain.Namespace("github.com/user/pkg@v1.0.0")
	seedArrow(t, r, domain.Arrow{Namespace: versioned, ArrowMeta: domain.ArrowMeta{Name: "Pkg"}})

	got, err := r.ResolveCatalogued(context.Background(), versioned)

	require.NoError(t, err)
	assert.Equal(t, versioned, got)
}

// Honouring a ref the catalog does not hold would send the runtime at an
// aggregate that cannot exist.
func TestResolveCatalogued_UnknownRefIsNotFound(t *testing.T) {
	r := newTestReader(t)
	seedArrow(t, r, domain.Arrow{
		Namespace: "github.com/user/pkg@v1.0.0",
		ArrowMeta: domain.ArrowMeta{Name: "Pkg"},
	})

	_, err := r.ResolveCatalogued(context.Background(), "github.com/user/pkg@v9.9.9")

	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestResolveCatalogued_UnknownArrowIsNotFound(t *testing.T) {
	r := newTestReader(t)

	_, err := r.ResolveCatalogued(context.Background(), "github.com/user/absent")

	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

// With several refs stored, the bare namespace resolves to the preferred one —
// the user-installed version — which is the same version the catalog uses to
// derive the arrow's own columns.
func TestResolveCatalogued_PrefersTheUserInstalledVersion(t *testing.T) {
	r := newTestReader(t)
	seedArrow(t, r, domain.Arrow{
		Namespace:     "github.com/user/pkg@v1.0.0",
		ArrowMeta:     domain.ArrowMeta{Name: "Pkg"},
		UserInstalled: false,
	})
	seedArrow(t, r, domain.Arrow{
		Namespace:     "github.com/user/pkg@v2.0.0",
		ArrowMeta:     domain.ArrowMeta{Name: "Pkg"},
		UserInstalled: true,
	})

	got, err := r.ResolveCatalogued(context.Background(), "github.com/user/pkg")

	require.NoError(t, err)
	assert.Equal(t, domain.Namespace("github.com/user/pkg@v2.0.0"), got)
}

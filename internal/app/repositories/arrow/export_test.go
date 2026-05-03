package arrow

import (
	"github.com/char2cs/asynx"

	arrowstore "github.com/rabbytesoftware/quiver/internal/app/repositories/arrow/internal/store"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
)

// NewTestable exposes arrowService construction for unit tests.
func NewTestable(
	r arrowstore.Store,
	axArrow asynx.Asynx[domain.Arrow],
	v vault.Vault,
	m manifold.Manifold,
) Arrow {
	return &arrowService{
		store:    r,
		axArrow:  axArrow,
		vault:    v,
		manifold: m,
	}
}

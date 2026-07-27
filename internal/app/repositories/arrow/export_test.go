package arrow

import (
	"github.com/char2cs/asynx"

	apphub "github.com/rabbytesoftware/quiver.core/internal/app/hub"
	arrowstore "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/store"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
)

// NewTestable exposes arrowService construction for unit tests. It registers no
// projections, so a test that only exercises one method is not made to drive
// the event loop.
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

// NewTestableProjecting builds an arrowService with its subscribers registered,
// so tests can observe the order the single projection runs things in.
func NewTestableProjecting(
	r arrowstore.Store,
	axArrow asynx.Asynx[domain.Arrow],
	v vault.Vault,
	m manifold.Manifold,
	hub apphub.WebSocketHub,
) (Arrow, error) {
	s := &arrowService{
		store:    r,
		axArrow:  axArrow,
		vault:    v,
		manifold: m,
		hub:      hub,
	}

	if err := s.registerProjections(); err != nil {
		return nil, err
	}

	return s, nil
}

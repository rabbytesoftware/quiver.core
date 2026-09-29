package cli

import (
	"context"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/ownership"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/platform"
)

type Placer interface {
	Place(
		ctx context.Context,
		req models.ApplyRequest,
		c models.Candidate,
	) (models.Placement, error)
	Remove(
		l platform.Layout,
		bare domain.Namespace,
		keep map[string]bool,
	) error
}

type placer struct {
	host    platform.Host
	bundles ownership.Bundles
}

func New(
	host platform.Host,
	bundles ownership.Bundles,
) Placer {
	return &placer{host: host, bundles: bundles}
}

func (p *placer) Place(
	ctx context.Context,
	req models.ApplyRequest,
	c models.Candidate,
) (models.Placement, error) {
	if p.host.GOOS == platform.GOOSWindows {
		return placeShim(req, c)
	}
	return p.placeSymlink(ctx, req, c)
}

func (p *placer) Remove(
	l platform.Layout,
	bare domain.Namespace,
	keep map[string]bool,
) error {
	if p.host.GOOS == platform.GOOSWindows {
		return removeShims(l, bare, keep)
	}
	return p.removeSymlinks(l, bare, keep)
}

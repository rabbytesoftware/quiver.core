package dmg

import (
	"context"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/guard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/models"
)

func NewWithMounter(
	maxBytes int64,
	attach func(ctx context.Context, image, mount string) error,
	detach func(ctx context.Context, mount string),
) models.Detect {
	return newWith(maxBytes, guard.HostRules{}, mounter{attach: attach, detach: detach})
}

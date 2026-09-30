//go:build !darwin

package dmg

import (
	"context"
	"fmt"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/models"
)

func hdiutil() mounter {
	return mounter{attach: attach, detach: detach}
}

func attach(
	_ context.Context,
	image string,
	_ string,
) error {
	return fmt.Errorf("unpack: dmg %s: %w", image, models.ErrUnsupportedPlatform)
}

func detach(
	_ context.Context,
	_ string,
) {
}

//go:build !windows

package msi

import (
	"context"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/models"
)

func msiexec(
	_ context.Context,
	_ invocation,
) (int, error) {
	return exitSuccess, models.ErrUnsupportedPlatform
}

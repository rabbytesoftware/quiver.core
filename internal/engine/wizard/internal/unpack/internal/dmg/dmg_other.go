//go:build !darwin

package dmg

import (
	"context"
	"fmt"
	"runtime"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/guard"
)

func Extract(
	_ context.Context,
	_ string,
	_ *guard.Guard,
) error {
	return fmt.Errorf("unpack: dmg unsupported on %s", runtime.GOOS)
}

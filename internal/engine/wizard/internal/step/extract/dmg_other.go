//go:build !darwin

package extract

import (
	"context"
	"fmt"
	"runtime"
)

func extractDmg(
	_ context.Context,
	_ string,
	_ *guard,
) error {
	return fmt.Errorf("extract: dmg unsupported on %s", runtime.GOOS)
}

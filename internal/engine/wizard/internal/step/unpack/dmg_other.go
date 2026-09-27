//go:build !darwin

package unpack

import (
	"context"
	"fmt"
	"runtime"
)

func ExtractDmg(
	_ context.Context,
	_ string,
	_ *Guard,
) error {
	return fmt.Errorf("unpack: dmg unsupported on %s", runtime.GOOS)
}

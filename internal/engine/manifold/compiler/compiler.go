package compiler

import (
	"errors"
	"fmt"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/models"
)

type Compiler interface {
	Compile(
		manifest *domain.Arrow,
		precompiled map[string]models.PrecompiledTarget,
		sel models.Selector,
	) error
}

type compiler struct{}

func New() Compiler {
	return &compiler{}
}

func (c *compiler) Compile(
	manifest *domain.Arrow,
	precompiled map[string]models.PrecompiledTarget,
	sel models.Selector,
) error {
	result := make(map[domain.OS]domain.Target)

	for _, os := range domain.AllOS() {
		target, err := sel.SelectTarget(precompiled, os)
		if err == nil {
			result[os] = target
			continue
		}

		if errors.Is(err, models.ErrNoTargetForOS) {
			continue
		}

		var ambiguous *models.AmbiguousTargetError
		if errors.As(err, &ambiguous) {
			return fmt.Errorf("ambiguous target for OS %s: %w", os, err)
		}

		return fmt.Errorf("compiler: select target for %s: %w", os, err)
	}

	manifest.Targets = result
	return nil
}

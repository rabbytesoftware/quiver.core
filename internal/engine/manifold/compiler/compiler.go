package compiler

import (
	"errors"
	"fmt"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
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

	addStartStop(result)
	manifest.Targets = result
	return nil
}

// addStartStop gives a manifest that exposes a desktop app, and declares no
// execute of its own, the same start and stop Fletcher synthesizes. Every
// target gets it, because a manifest cannot mix targets with and without an
// execute. An explicit execute always wins.
func addStartStop(
	targets map[domain.OS]domain.Target,
) {
	hasApp := false
	for _, t := range targets {
		if len(t.Lifecycle.Execute) > 0 {
			return
		}
		hasApp = hasApp || len(t.Expose.Desktop) > 0
	}
	if !hasApp {
		return
	}
	for os, t := range targets {
		command := `exec "${ARROW_APP}"`
		if os.IsWindows() {
			command = `"${ARROW_APP}"`
		}
		t.Lifecycle.Execute = step.StepList{step.NewRunStep("Start", command, false, "", true)}
		t.Lifecycle.Stop = step.StepList{step.NewSignalStep("Stop", step.SignalKindGraceful, "10s", false)}
		targets[os] = t
	}
}

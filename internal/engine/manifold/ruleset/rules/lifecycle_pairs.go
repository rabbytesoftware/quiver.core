package rules

import (
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/ruleset/aerrors"
)

type LifecyclePairsRule struct{}

func (LifecyclePairsRule) Name() string { return "lifecycle_pairs" }

func (LifecyclePairsRule) Validate(
	m *domain.Arrow,
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors
	for os, target := range m.Targets {
		errs = append(errs, checkLifecyclePairs(string(os), target)...)
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

func checkLifecyclePairs(
	key string,
	target domain.Target,
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors

	hasInstall := len(target.Lifecycle.Install) > 0
	hasUninstall := len(target.Lifecycle.Uninstall) > 0
	if hasInstall != hasUninstall {
		errs = append(errs, aerrors.RuleError{
			Field:   fmt.Sprintf("targets[%s].lifecycle.install", key),
			Rule:    "missing_pair",
			Message: "install and uninstall must both be defined or both be empty",
		})
	}

	hasExecute := len(target.Lifecycle.Execute) > 0
	hasStop := len(target.Lifecycle.Stop) > 0
	// stop without execute is always invalid (nothing to stop).
	// execute without stop is valid for tools that run and exit on their own.
	if hasStop && !hasExecute {
		errs = append(errs, aerrors.RuleError{
			Field:   fmt.Sprintf("targets[%s].lifecycle.stop", key),
			Rule:    "missing_pair",
			Message: "stop requires execute to also be defined",
		})
	}

	return errs
}

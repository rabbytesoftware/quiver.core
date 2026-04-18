package rules

import (
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/ruleset/aerrors"
)

type LifecyclePairsRule struct{}

func (LifecyclePairsRule) Name() string { return "lifecycle_pairs" }

func (LifecyclePairsRule) Validate(
	m *domain.ArrowManifest,
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

	hasInstall := target.Lifecycle.Install != nil
	hasUninstall := target.Lifecycle.Uninstall != nil
	if hasInstall != hasUninstall {
		errs = append(errs, aerrors.RuleError{
			Field:   fmt.Sprintf("targets[%s].lifecycle.install", key),
			Rule:    "missing_pair",
			Message: "install and uninstall must both be defined or both be empty",
		})
	}

	hasExecute := target.Lifecycle.Execute != nil
	hasStop := target.Lifecycle.Stop != nil
	if hasExecute != hasStop {
		errs = append(errs, aerrors.RuleError{
			Field:   fmt.Sprintf("targets[%s].lifecycle.execute", key),
			Rule:    "missing_pair",
			Message: "execute and stop must both be defined or both be empty",
		})
	}

	return errs
}

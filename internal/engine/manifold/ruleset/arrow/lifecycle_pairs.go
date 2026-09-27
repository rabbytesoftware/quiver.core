package arrow

import (
	"fmt"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/ruleset/aerrors"
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
	missingUninstall := hasInstall && !hasUninstall && !isWorkdirOnlyInstall(target.Lifecycle.Install)
	missingInstall := hasUninstall && !hasInstall
	if missingUninstall || missingInstall {
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

func isWorkdirOnlyInstall(
	steps step.StepList,
) bool {
	for _, s := range steps {
		if !isWorkdirAnchoredPlacement(s) {
			return false
		}
	}
	return true
}

func isWorkdirAnchoredPlacement(
	s step.Step,
) bool {
	switch v := s.(type) {
	case step.FetchStep:
		return isWorkdirAnchoredTo(v.To)
	case *step.FetchStep:
		return isWorkdirAnchoredTo(v.To)
	case step.ExtractStep:
		return isWorkdirAnchoredTo(v.To)
	case *step.ExtractStep:
		return isWorkdirAnchoredTo(v.To)
	case step.PortableStep:
		return isWorkdirAnchoredTo(v.To)
	case *step.PortableStep:
		return isWorkdirAnchoredTo(v.To)
	default:
		return false
	}
}

func isWorkdirAnchoredTo(
	to step.Overrideable[string],
) bool {
	if !isWorkdirAnchoredValue(to.Default) {
		return false
	}
	for _, v := range to.OSArch {
		if !isWorkdirAnchoredValue(v) {
			return false
		}
	}
	return true
}

func isWorkdirAnchoredValue(
	v string,
) bool {
	if containsPathTraversal(v) {
		return false
	}
	return isAnchoredByPrefix(v, "${INSTALL_PATH}") || isAnchoredByPrefix(v, "${WORKDIR}")
}

func isAnchoredByPrefix(
	v string,
	prefix string,
) bool {
	if v == prefix {
		return true
	}
	return strings.HasPrefix(v, prefix+"/")
}

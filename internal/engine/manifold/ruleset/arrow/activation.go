package arrow

import (
	"fmt"
	"slices"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/ruleset/aerrors"
)

type ActivationRule struct{}

func (ActivationRule) Name() string { return "activation" }

func (ActivationRule) Validate(
	m *domain.Arrow,
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors
	for os, target := range m.Targets {
		errs = append(errs, checkActivation(string(os), target)...)
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

func checkActivation(
	key string,
	target domain.Target,
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors
	for method, mode := range target.Activation {
		field := fmt.Sprintf("targets[%s].activation[%s]", key, method)
		if !slices.Contains(domain.ActivationMethods(), method) {
			errs = append(errs, aerrors.RuleError{
				Field:   field,
				Rule:    "invalid_activation_method",
				Message: fmt.Sprintf("activation names method %q (must be one of %v)", method, domain.ActivationMethods()),
			})
		}
		if mode != domain.ActivationRestart {
			errs = append(errs, aerrors.RuleError{
				Field:   field,
				Rule:    "invalid_activation_value",
				Message: fmt.Sprintf("method %q has activation %q (must be %s)", method, mode, domain.ActivationRestart),
			})
		}
	}
	return errs
}

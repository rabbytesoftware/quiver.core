package rules

import (
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/ruleset/aerrors"
)

var validMethodStates = map[string]bool{
	string(domain.ArrowStateReady):   true,
	string(domain.ArrowStateRunning): true,
}

type MethodStatesRule struct{}

func (MethodStatesRule) Name() string { return "method_states" }

func (MethodStatesRule) Validate(
	m *domain.Arrow,
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors
	for os, target := range m.Targets {
		errs = append(errs, checkMethodStates(string(os), target)...)
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

func checkMethodStates(
	key string,
	target domain.Target,
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors
	for name, m := range target.Methods {
		errs = append(errs, checkMethodAvailableIn(key, name, m)...)
	}
	return errs
}

func checkMethodAvailableIn(
	key string,
	name string,
	m domain.Method,
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors
	for _, state := range m.AvailableIn {
		if validMethodStates[string(state)] {
			continue
		}
		errs = append(errs, aerrors.RuleError{
			Field:   fmt.Sprintf("targets[%s].methods[%s].available_in", key, name),
			Rule:    "invalid_state",
			Message: fmt.Sprintf("method %q has invalid state %q (must be ready or running)", name, state),
		})
	}
	return errs
}

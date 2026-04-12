package assembler

import (
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/domain/netbridge"
)

func validateLifecyclePairs(lc domain.Lifecycle) AssemblerErrors {
	var errs AssemblerErrors

	hasInstall := len(lc.Install) > 0
	hasUninstall := len(lc.Uninstall) > 0
	if hasInstall != hasUninstall {
		errs = append(errs, AssemblerError{
			Field:   "lifecycle.install",
			Rule:    "missing_pair",
			Message: "install and uninstall must both be defined or both be empty",
		})
	}

	hasExecute := len(lc.Execute) > 0
	hasStop := len(lc.Stop) > 0
	if hasExecute != hasStop {
		errs = append(errs, AssemblerError{
			Field:   "lifecycle.execute",
			Rule:    "missing_pair",
			Message: "execute and stop must both be defined or both be empty",
		})
	}

	return errs
}

func validateDependencies(deps []domain.Namespace) AssemblerErrors {
	var errs AssemblerErrors
	for i, d := range deps {
		if err := d.Validate(); err != nil {
			errs = append(errs, AssemblerError{
				Field:   fmt.Sprintf("dependencies[%d]", i),
				Rule:    "invalid_namespace",
				Message: fmt.Sprintf("invalid dependency %q: %v", d, err),
			})
		}
	}
	return errs
}

func validateVariables(vars []domain.Variable) AssemblerErrors {
	var errs AssemblerErrors
	names := make([]string, 0, len(vars))

	for i, v := range vars {
		if err := v.Validate(); err != nil {
			errs = append(errs, AssemblerError{
				Field:   fmt.Sprintf("variables[%d]", i),
				Rule:    "invalid_variable",
				Message: err.Error(),
			})
		}

		if v.Type.IsSelect() && len(v.Values) == 0 {
			errs = append(errs, AssemblerError{
				Field:   fmt.Sprintf("variables[%d].values", i),
				Rule:    "missing_values",
				Message: fmt.Sprintf("select variable %q must have at least one value", v.Name),
			})
		}

		names = append(names, v.Name)
	}

	errs = append(errs, checkDuplicates(names, "variables", "duplicate_name")...)

	return errs
}

func validateNetbridge(ports []netbridge.PortDef) AssemblerErrors {
	var errs AssemblerErrors
	names := make([]string, 0, len(ports))

	for i, p := range ports {
		if err := p.Validate(); err != nil {
			errs = append(errs, AssemblerError{
				Field:   fmt.Sprintf("netbridge[%d]", i),
				Rule:    "invalid_port",
				Message: err.Error(),
			})
		}
		names = append(names, p.Name)
	}

	errs = append(errs, checkDuplicates(names, "netbridge", "duplicate_name")...)

	return errs
}

func validateMethodStates(methods map[string]domain.Method) AssemblerErrors {
	validStates := map[string]struct{}{
		string(domain.ArrowStateReady):   {},
		string(domain.ArrowStateRunning): {},
	}

	var errs AssemblerErrors
	for name, m := range methods {
		for _, state := range m.AvailableIn {
			if _, ok := validStates[string(state)]; !ok {
				errs = append(errs, AssemblerError{
					Field:   fmt.Sprintf("methods[%s].available_in", name),
					Rule:    "invalid_state",
					Message: fmt.Sprintf("method %q has invalid state %q (must be ready or running)", name, state),
				})
			}
		}
	}
	return errs
}

func checkDuplicates(names []string, field, rule string) AssemblerErrors {
	seen := make(map[string]bool, len(names))
	reported := make(map[string]bool)
	var errs AssemblerErrors
	for _, name := range names {
		if seen[name] && !reported[name] {
			errs = append(errs, AssemblerError{
				Field:   field,
				Rule:    rule,
				Message: fmt.Sprintf("duplicate name %q", name),
			})
			reported[name] = true
		}
		seen[name] = true
	}
	return errs
}

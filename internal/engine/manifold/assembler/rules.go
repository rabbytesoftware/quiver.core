package assembler

import (
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/domain/netbridge"
)

func checkDuplicates(names []string, itemType string) error {
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if _, exists := seen[name]; exists {
			return fmt.Errorf("%w: duplicate %s %q", ErrInvalidManifest, itemType, name)
		}
		seen[name] = struct{}{}
	}
	return nil
}

func validateLifecyclePairs(
	lc domain.Lifecycle,
) error {
	hasInstall := len(lc.Install) > 0
	hasUninstall := len(lc.Uninstall) > 0

	if hasInstall != hasUninstall {
		return fmt.Errorf("%w: install and uninstall must both be defined or both be empty", ErrInvalidManifest)
	}

	hasExecute := len(lc.Execute) > 0
	hasStop := len(lc.Stop) > 0

	if hasExecute != hasStop {
		return fmt.Errorf("%w: execute and stop must both be defined or both be empty", ErrInvalidManifest)
	}

	return nil
}

func validateDependencies(
	deps []domain.Namespace,
) error {
	for _, d := range deps {
		if err := d.Validate(); err != nil {
			return fmt.Errorf("%w: invalid dependency %q: %w", ErrInvalidManifest, d, err)
		}
	}

	return nil
}

func validateVariables(
	vars []domain.Variable,
) error {
	names := make([]string, len(vars))

	for i, v := range vars {
		if err := v.Validate(); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidManifest, err)
		}

		names[i] = v.Name

		vt := v.Type
		if vt.IsSelect() && len(v.Values) == 0 {
			return fmt.Errorf("%w: select variable %q must have values", ErrInvalidManifest, v.Name)
		}
	}

	return checkDuplicates(names, "variable name")
}

func validateNetbridge(
	ports []netbridge.PortDef,
) error {
	names := make([]string, len(ports))

	for i, p := range ports {
		if err := p.Validate(); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidManifest, err)
		}

		names[i] = p.Name
	}

	return checkDuplicates(names, "netbridge port name")
}

func validateMethodStates(
	methods map[string]domain.Method,
) error {
	validStates := map[string]struct{}{
		string(domain.ArrowStateReady):   {},
		string(domain.ArrowStateRunning): {},
	}

	for name, m := range methods {
		for _, state := range m.AvailableIn {
			if _, ok := validStates[string(state)]; !ok {
				return fmt.Errorf("%w: method %q has invalid state %q (must be ready or running)",
					ErrInvalidManifest, name, state)
			}
		}
	}

	return nil
}

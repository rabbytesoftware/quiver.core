package assembler

import (
	"fmt"
	"slices"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/domain/netbridge"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/models"
)

func checkOSCompatibility(
	os string,
	supportedOS []string,
) error {
	if !slices.Contains(supportedOS, os) {
		return ErrUnsupportedPlatform
	}

	return nil
}

func (a *Assembler) resolveOverrides(
	steps []models.RawStep,
	os string,
	supportedOS []string,
) ([]models.RawStep, error) {
	result := make([]models.RawStep, len(steps))
	copy(result, steps)

	for i, s := range result {
		if len(s.Overrides) == 0 {
			continue
		}

		handler, ok := a.stepHandlers[s.Type]
		if !ok || !handler.SupportsOverrides() {
			return nil, fmt.Errorf("%w: overrides are only allowed on steps that support them", ErrInvalidManifest)
		}

		for overrideOS := range s.Overrides {
			if !slices.Contains(supportedOS, overrideOS) {
				return nil, fmt.Errorf("%w: override OS %q not in requirements.os", ErrInvalidManifest, overrideOS)
			}
		}

		override, ok := s.Overrides[os]
		if !ok {
			result[i].Overrides = nil
			continue
		}

		if override.Command != "" {
			result[i].Command = override.Command
		}
		if override.Title != "" {
			result[i].Title = override.Title
		}
		if override.Timeout != "" {
			result[i].Timeout = override.Timeout
		}
		if override.ExitOnFailure != nil {
			result[i].ExitOnFailure = override.ExitOnFailure
		}
		result[i].Overrides = nil
	}

	return result, nil
}

func validateLifecyclePairs(
	lc models.RawLifecycle,
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

func (a *Assembler) validateStep(
	s models.RawStep,
) error {
	handler, ok := a.stepHandlers[s.Type]
	if !ok {
		return fmt.Errorf("%w: unknown step type %q", ErrInvalidManifest, s.Type)
	}

	if err := handler.Validate(s); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidManifest, err)
	}

	return nil
}

func (a *Assembler) resolveAllOverrides(
	lc models.RawLifecycle,
	methods map[string]models.RawMethod,
	os string,
	supportedOS []string,
) (models.RawLifecycle, map[string]models.RawMethod, error) {
	install, err := a.resolveOverrides(lc.Install, os, supportedOS)
	if err != nil {
		return models.RawLifecycle{}, nil, err
	}

	execute, err := a.resolveOverrides(lc.Execute, os, supportedOS)
	if err != nil {
		return models.RawLifecycle{}, nil, err
	}

	stop, err := a.resolveOverrides(lc.Stop, os, supportedOS)
	if err != nil {
		return models.RawLifecycle{}, nil, err
	}

	uninstall, err := a.resolveOverrides(lc.Uninstall, os, supportedOS)
	if err != nil {
		return models.RawLifecycle{}, nil, err
	}

	resolvedLC := models.RawLifecycle{
		Install:   install,
		Execute:   execute,
		Stop:      stop,
		Uninstall: uninstall,
	}

	resolvedMethods := make(map[string]models.RawMethod, len(methods))
	for name, m := range methods {
		steps, err := a.resolveOverrides(m.Steps, os, supportedOS)
		if err != nil {
			return models.RawLifecycle{}, nil, err
		}

		resolvedMethods[name] = models.RawMethod{
			AvailableIn: m.AvailableIn,
			Steps:       steps,
		}
	}

	return resolvedLC, resolvedMethods, nil
}

func (a *Assembler) validateArrow(
	raw *models.RawArrow,
	resolvedLC models.RawLifecycle,
	resolvedMethods map[string]models.RawMethod,
) error {
	if err := validateLifecyclePairs(resolvedLC); err != nil {
		return err
	}

	for _, steps := range [][]models.RawStep{
		resolvedLC.Install,
		resolvedLC.Execute,
		resolvedLC.Stop,
		resolvedLC.Uninstall,
	} {
		if err := a.validateSteps(steps); err != nil {
			return err
		}
	}

	for _, m := range resolvedMethods {
		if err := a.validateSteps(m.Steps); err != nil {
			return err
		}
	}

	if err := validateVariables(raw.Variables); err != nil {
		return err
	}

	if err := validateNetbridge(raw.Netbridge); err != nil {
		return err
	}

	return validateMethodStates(resolvedMethods)
}

func validateDependencies(
	deps []string,
) ([]domain.Namespace, error) {
	namespaces := make([]domain.Namespace, 0, len(deps))

	for _, d := range deps {
		ns := domain.Namespace(d)
		if err := ns.Validate(); err != nil {
			return nil, fmt.Errorf("%w: invalid dependency %q: %v", ErrInvalidManifest, d, err)
		}
		namespaces = append(namespaces, ns)
	}

	return namespaces, nil
}

func validateVariables(
	vars []domain.Variable,
) error {
	seen := make(map[string]struct{}, len(vars))

	for _, v := range vars {
		if err := v.Validate(); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidManifest, err)
		}

		if _, exists := seen[v.Name]; exists {
			return fmt.Errorf("%w: duplicate variable name %q", ErrInvalidManifest, v.Name)
		}
		seen[v.Name] = struct{}{}

		vt := v.Type
		if vt.IsSelect() && len(v.Values) == 0 {
			return fmt.Errorf("%w: select variable %q must have values", ErrInvalidManifest, v.Name)
		}
	}

	return nil
}

func validateNetbridge(
	ports []netbridge.PortDef,
) error {
	seen := make(map[string]struct{}, len(ports))

	for _, p := range ports {
		if err := p.Validate(); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidManifest, err)
		}

		if _, exists := seen[p.Name]; exists {
			return fmt.Errorf("%w: duplicate netbridge port name %q", ErrInvalidManifest, p.Name)
		}
		seen[p.Name] = struct{}{}
	}

	return nil
}

func validateMethodStates(
	methods map[string]models.RawMethod,
) error {
	validStates := map[string]struct{}{
		string(domain.ArrowStateReady):   {},
		string(domain.ArrowStateRunning): {},
	}

	for name, m := range methods {
		for _, state := range m.AvailableIn {
			if _, ok := validStates[state]; !ok {
				return fmt.Errorf("%w: method %q has invalid state %q (must be ready or running)",
					ErrInvalidManifest, name, state)
			}
		}
	}

	return nil
}

func (a *Assembler) validateSteps(
	steps []models.RawStep,
) error {
	for _, s := range steps {
		if err := a.validateStep(s); err != nil {
			return err
		}
	}

	return nil
}

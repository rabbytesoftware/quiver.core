package assembler

import (
	"fmt"
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
	dstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/models"
)

func (a *Assembler) buildSteps(
	raw []models.RawStep,
) ([]dstep.Step, error) {
	steps := make([]dstep.Step, 0, len(raw))

	for _, s := range raw {
		built, err := a.buildStep(s)
		if err != nil {
			return nil, err
		}
		steps = append(steps, built)
	}

	return steps, nil
}

func (a *Assembler) buildStep(
	s models.RawStep,
) (dstep.Step, error) {
	handler, ok := a.stepHandlers[s.Type]
	if !ok {
		return nil, fmt.Errorf("%w: unknown step type %q", ErrInvalidManifest, s.Type)
	}

	timeout, err := parseDuration(s.Timeout)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid timeout %q: %v", ErrInvalidManifest, s.Timeout, err)
	}

	exitOnFailure := resolveExitOnFailure(s.ExitOnFailure)

	return handler.Build(s, timeout, exitOnFailure)
}

func (a *Assembler) buildLifecycle(
	raw models.RawLifecycle,
) (domain.Lifecycle, error) {
	install, err := a.buildSteps(raw.Install)
	if err != nil {
		return domain.Lifecycle{}, fmt.Errorf("install: %w", err)
	}

	execute, err := a.buildSteps(raw.Execute)
	if err != nil {
		return domain.Lifecycle{}, fmt.Errorf("execute: %w", err)
	}

	stop, err := a.buildSteps(raw.Stop)
	if err != nil {
		return domain.Lifecycle{}, fmt.Errorf("stop: %w", err)
	}

	uninstall, err := a.buildSteps(raw.Uninstall)
	if err != nil {
		return domain.Lifecycle{}, fmt.Errorf("uninstall: %w", err)
	}

	return domain.Lifecycle{
		Install:   install,
		Execute:   execute,
		Stop:      stop,
		Uninstall: uninstall,
	}, nil
}

func (a *Assembler) buildMethods(
	raw map[string]models.RawMethod,
) (map[string]domain.Method, error) {
	methods := make(map[string]domain.Method, len(raw))

	for name, m := range raw {
		steps, err := a.buildSteps(m.Steps)
		if err != nil {
			return nil, fmt.Errorf("method %q: %w", name, err)
		}

		states := make([]domain.ArrowState, 0, len(m.AvailableIn))
		for _, s := range m.AvailableIn {
			states = append(states, domain.ArrowState(s))
		}

		methods[name] = domain.Method{
			AvailableIn: states,
			Steps:       steps,
		}
	}

	return methods, nil
}

func buildRequirement(
	raw models.RawRequirements,
) (domain.Requirement, error) {
	osSlice := make([]domain.OS, 0, len(raw.OS))
	for _, o := range raw.OS {
		osSlice = append(osSlice, domain.OS(o))
	}

	req := domain.Requirement{
		CpuCores: raw.CpuCores,
		MemoryGB: raw.RamGB,
		DiskGB:   raw.DiskGB,
		OS:       osSlice,
	}

	if err := req.Validate(); err != nil {
		return domain.Requirement{}, fmt.Errorf("%w: %v", ErrInvalidManifest, err)
	}

	return req, nil
}

func (a *Assembler) buildArrow(
	raw *models.RawArrow,
	resolvedLC models.RawLifecycle,
	resolvedMethods map[string]models.RawMethod,
) (*domain.ArrowManifest, error) {
	deps, err := validateDependencies(raw.Dependencies)
	if err != nil {
		return nil, err
	}

	lifecycle, err := a.buildLifecycle(resolvedLC)
	if err != nil {
		return nil, err
	}

	methods, err := a.buildMethods(resolvedMethods)
	if err != nil {
		return nil, err
	}

	req, err := buildRequirement(raw.Requirements)
	if err != nil {
		return nil, err
	}

	return &domain.ArrowManifest{
		Name:         raw.Metadata.Name,
		Description:  raw.Metadata.Description,
		Version:      raw.Metadata.Version,
		License:      raw.Metadata.License,
		URL:          raw.Metadata.Quiver,
		Maintainers:  raw.Metadata.Maintainers,
		Credits:      raw.Metadata.Credits,
		Tags:         raw.Metadata.Tags,
		Requirements: req,
		Dependencies: deps,
		Variables:    raw.Variables,
		Netbridge:    raw.Netbridge,
		Lifecycle:    lifecycle,
		Methods:      methods,
	}, nil
}

func parseDuration(
	s string,
) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}

	return time.ParseDuration(s)
}

func resolveExitOnFailure(
	v *bool,
) bool {
	if v == nil {
		return true
	}

	return *v
}

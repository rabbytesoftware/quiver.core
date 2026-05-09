package v0

import (
	_ "embed"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/domain/netbridge"
	"github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/models"
)

//go:embed schema.json
var schemaJSON []byte

func Map(
	data []byte,
) (*domain.Arrow, map[string]models.PrecompiledTarget, error) {
	var raw arrowV0
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal arrow@v0 YAML: %w", err)
	}
	return toAggregate(raw)
}

func toAggregate(raw arrowV0) (*domain.Arrow, map[string]models.PrecompiledTarget, error) {
	if len(raw.Targets) == 0 {
		return nil, nil, fmt.Errorf(
			"this manifest uses the pre-refactor arrow@v0 shape (no \"targets:\" section); " +
				"rewrite it according to docs/spec/arrow/v0/manifest.md — " +
				"no migration shim is provided, v0 is still in development",
		)
	}

	precompiled, err := toTargets(raw.Targets)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid targets: %w", err)
	}

	manifest := &domain.Arrow{
		ArrowMeta: domain.ArrowMeta{
			Name:        raw.Metadata.Name,
			Description: raw.Metadata.Description,
			Version:     raw.Metadata.Version,
			License:     raw.Metadata.License,
			URL:         raw.Metadata.URL,
			Maintainers: toCredits(raw.Metadata.Maintainers),
			Credits:     toCredits(raw.Metadata.Credits),
			Tags:        raw.Metadata.Tags,
		},
		Variables: toVariables(raw.Variables),
		Netbridge: toPorts(raw.Netbridge),
	}

	return manifest, precompiled, nil
}

func toTargets(rawTargets map[string]targetV0) (map[string]models.PrecompiledTarget, error) {
	result := make(map[string]models.PrecompiledTarget, len(rawTargets))
	for name, t := range rawTargets {
		target, err := toTarget(t)
		if err != nil {
			return nil, fmt.Errorf("invalid target %q: %w", name, err)
		}
		result[name] = target
	}
	return result, nil
}

func toTarget(t targetV0) (models.PrecompiledTarget, error) {
	tools := toNamespaces(t.Tools)
	services := toNamespaces(t.Services)

	lifecycle, err := toLifecycle(t.Lifecycle)
	if err != nil {
		return models.PrecompiledTarget{}, fmt.Errorf("invalid lifecycle: %w", err)
	}

	methods, err := toMethods(t.Methods)
	if err != nil {
		return models.PrecompiledTarget{}, fmt.Errorf("invalid methods: %w", err)
	}

	return models.PrecompiledTarget{
		Base:         t.Base,
		Requirements: toRequirement(t.Requirements),
		Tools:        tools,
		Services:     services,
		Exports:      toExports(t.Exports),
		Lifecycle:    lifecycle,
		Methods:      methods,
	}, nil
}

func toRequirement(req requirementsV0) domain.Requirement {
	r := domain.Requirement{}
	if req.CpuCores != nil {
		r.CpuCores = *req.CpuCores
	}
	if req.RamGB != nil {
		r.MemoryGB = *req.RamGB
	}
	if req.DiskGB != nil {
		r.DiskGB = *req.DiskGB
	}
	return r
}

func toNamespaces(refs []string) []domain.Namespace {
	if refs == nil {
		return nil
	}
	result := make([]domain.Namespace, len(refs))
	for i, r := range refs {
		result[i] = domain.Namespace(r)
	}
	return result
}

func toExports(exports map[string]overrideableV0[string]) map[string]step.Overrideable[string] {
	if exports == nil {
		return nil
	}
	result := make(map[string]step.Overrideable[string], len(exports))
	for k, v := range exports {
		result[k] = toStepOverrideable(v)
	}
	return result
}

func toCredits(credits []creditV0) []domain.Credit {
	result := make([]domain.Credit, len(credits))
	for i, c := range credits {
		result[i] = domain.Credit{
			Name:  c.Name,
			Email: c.Email,
			URL:   c.URL,
		}
	}
	return result
}

func toVariables(vars []variableV0) []domain.Variable {
	result := make([]domain.Variable, len(vars))
	for i, v := range vars {
		result[i] = domain.Variable{
			Name:        v.Name,
			Description: v.Description,
			Default:     v.Default,
			Sensitive:   v.Sensitive,
			Values:      v.Values,
			Min:         v.Min,
			Max:         v.Max,
			Type:        domain.VariableType(v.Type),
		}
	}
	return result
}

func toPorts(ports []portV0) []netbridge.PortDef {
	result := make([]netbridge.PortDef, len(ports))
	for i, p := range ports {
		result[i] = netbridge.PortDef{
			Name:     p.Name,
			Protocol: netbridge.Protocol(p.Protocol),
			Default:  p.Default,
			Required: p.Required,
		}
	}
	return result
}

func toLifecycle(lc lifecycleV0) (domain.TargetLifecycle, error) {
	install, err := toStepList(lc.Install)
	if err != nil {
		return domain.TargetLifecycle{}, fmt.Errorf("invalid install steps: %w", err)
	}

	update, err := toStepList(lc.Update)
	if err != nil {
		return domain.TargetLifecycle{}, fmt.Errorf("invalid update steps: %w", err)
	}

	execute, err := toStepList(lc.Execute)
	if err != nil {
		return domain.TargetLifecycle{}, fmt.Errorf("invalid execute steps: %w", err)
	}

	stop, err := toStepList(lc.Stop)
	if err != nil {
		return domain.TargetLifecycle{}, fmt.Errorf("invalid stop steps: %w", err)
	}

	uninstall, err := toStepList(lc.Uninstall)
	if err != nil {
		return domain.TargetLifecycle{}, fmt.Errorf("invalid uninstall steps: %w", err)
	}

	return domain.TargetLifecycle{
		Install:   install,
		Update:    update,
		Execute:   execute,
		Stop:      stop,
		Uninstall: uninstall,
	}, nil
}

func toStepList(steps []stepV0) (step.StepList, error) {
	if steps == nil {
		return nil, nil
	}
	result := make(step.StepList, 0, len(steps))
	for _, s := range steps {
		st, err := toStep(s)
		if err != nil {
			return nil, err
		}
		result = append(result, st)
	}
	return result, nil
}

func toStep(s stepV0) (step.Step, error) {
	exitOnFailure := resolveExitOnFailure(s.ExitOnFailure)

	switch s.Type {
	case "run":
		st := step.NewRunStep(
			s.Title,
			s.Command.Default,
			s.Elevated.Default,
			s.Timeout.Default,
			exitOnFailure,
		)
		st.Command = toStepOverrideable(s.Command)
		st.Elevated = toStepOverrideableBool(s.Elevated)
		st.Timeout = toStepOverrideable(s.Timeout)
		return st, nil

	case "fetch":
		st := step.NewFetchStep(
			s.Title,
			s.URL.Default,
			s.To.Default,
			s.Checksum.Default,
			s.Timeout.Default,
			exitOnFailure,
		)
		st.URL = toStepOverrideable(s.URL)
		st.To = toStepOverrideable(s.To)
		st.Checksum = toStepOverrideable(s.Checksum)
		st.Timeout = toStepOverrideable(s.Timeout)
		return st, nil

	case "signal":
		st := step.NewSignalStep(
			s.Title,
			step.SignalKind(s.Signal.Default),
			s.Timeout.Default,
			exitOnFailure,
		)
		st.Signal = toSignalOverrideable(s.Signal)
		st.Timeout = toStepOverrideable(s.Timeout)
		return st, nil

	case "dependencies":
		return nil, fmt.Errorf("step type \"dependencies\" is synthetic and must not appear in manifests")

	default:
		return nil, fmt.Errorf("unknown step type: %q", s.Type)
	}
}

func toMethods(methods map[string]methodV0) (map[string]domain.Method, error) {
	result := make(map[string]domain.Method)
	for name, m := range methods {
		states := make([]domain.ArrowState, len(m.AvailableIn))
		for i, s := range m.AvailableIn {
			states[i] = domain.ArrowState(s)
		}

		steps, err := toStepList(m.Steps)
		if err != nil {
			return nil, fmt.Errorf("invalid steps in method %q: %w", name, err)
		}

		result[name] = domain.Method{
			AvailableIn: states,
			Steps:       steps,
		}
	}
	return result, nil
}

func resolveExitOnFailure(v *bool) bool {
	if v == nil {
		return true
	}
	return *v
}

func toStepOverrideable(o overrideableV0[string]) step.Overrideable[string] {
	return step.Overrideable[string]{
		Default: o.Default,
		OSArch:  o.OSArch,
	}
}

func toStepOverrideableBool(o overrideableV0[bool]) step.Overrideable[bool] {
	return step.Overrideable[bool]{
		Default: o.Default,
		OSArch:  o.OSArch,
	}
}

func toSignalOverrideable(o overrideableV0[string]) step.Overrideable[step.SignalKind] {
	result := step.Overrideable[step.SignalKind]{
		Default: step.SignalKind(o.Default),
	}
	if len(o.OSArch) > 0 {
		result.OSArch = make(map[string]step.SignalKind, len(o.OSArch))
		for k, v := range o.OSArch {
			result.OSArch[k] = step.SignalKind(v)
		}
	}
	return result
}

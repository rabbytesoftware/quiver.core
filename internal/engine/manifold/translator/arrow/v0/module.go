package v0

import (
	_ "embed"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/domain/netbridge"
	"github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
)

//go:embed schema.json
var schemaJSON []byte

var Default = &module{}

type module struct{}

func (m *module) Version() string { return "v0" }

func (m *module) GetSchema() ([]byte, error) { return schemaJSON, nil }

func (m *module) Map(data []byte) (*domain.ArrowManifest, error) {
	var raw arrowV0
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal arrow@v0 YAML: %w", err)
	}
	return toAggregate(raw)
}

func toAggregate(raw arrowV0) (*domain.ArrowManifest, error) {
	deps, err := toDependencies(raw.Dependencies)
	if err != nil {
		return nil, fmt.Errorf("invalid dependencies: %w", err)
	}

	lifecycle, err := toLifecycle(raw.Lifecycle)
	if err != nil {
		return nil, fmt.Errorf("invalid lifecycle: %w", err)
	}

	methods, err := toMethods(raw.Methods)
	if err != nil {
		return nil, fmt.Errorf("invalid methods: %w", err)
	}

	return &domain.ArrowManifest{
		Name:         raw.Name,
		Description:  raw.Description,
		Version:      raw.Version,
		License:      raw.License,
		URL:          raw.URL,
		Maintainers:  raw.Maintainers,
		Tags:         raw.Tags,
		Credits:      toCredits(raw.Credits),
		Requirements: toRequirements(raw.Requirements),
		Dependencies: deps,
		Variables:    toVariables(raw.Variables),
		Netbridge:    toPorts(raw.Netbridge),
		Lifecycle:    lifecycle,
		Methods:      methods,
	}, nil
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

func toRequirements(req requirementsV0) domain.Requirement {
	os := make([]domain.OS, len(req.OS))
	for i, o := range req.OS {
		os[i] = domain.OS(o)
	}
	return domain.Requirement{
		CpuCores: req.CpuCores,
		MemoryGB: req.RamGB,
		DiskGB:   req.DiskGB,
		OS:       os,
	}
}

func toDependencies(deps []string) ([]domain.Namespace, error) {
	result := make([]domain.Namespace, len(deps))
	for i, d := range deps {
		result[i] = domain.Namespace(d)
	}
	return result, nil
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

func toLifecycle(lc lifecycleV0) (domain.Lifecycle, error) {
	install, err := toStepList(lc.Install)
	if err != nil {
		return domain.Lifecycle{}, fmt.Errorf("invalid install steps: %w", err)
	}

	execute, err := toStepList(lc.Execute)
	if err != nil {
		return domain.Lifecycle{}, fmt.Errorf("invalid execute steps: %w", err)
	}

	stop, err := toStepList(lc.Stop)
	if err != nil {
		return domain.Lifecycle{}, fmt.Errorf("invalid stop steps: %w", err)
	}

	uninstall, err := toStepList(lc.Uninstall)
	if err != nil {
		return domain.Lifecycle{}, fmt.Errorf("invalid uninstall steps: %w", err)
	}

	return domain.Lifecycle{
		Install:   install,
		Execute:   execute,
		Stop:      stop,
		Uninstall: uninstall,
	}, nil
}

func toStepList(steps []stepV0) (step.StepList, error) {
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
	timeout, err := parseTimeout(s.Timeout.Default)
	if err != nil {
		return nil, fmt.Errorf("invalid timeout %q: %w", s.Timeout.Default, err)
	}

	switch s.Type {
	case "run":
		st := step.NewRunStep(s.Title, s.Command.Default, timeout, s.ExitOnFailure)
		st.Command = toStepOverrideable(s.Command)
		st.Timeout = toStepOverrideable(s.Timeout)
		return st, nil

	case "fetch":
		st := step.NewFetchStep(s.Title, s.URL.Default, s.To.Default, timeout, s.ExitOnFailure)
		st.URL = toStepOverrideable(s.URL)
		st.To = toStepOverrideable(s.To)
		st.Timeout = toStepOverrideable(s.Timeout)
		return st, nil

	case "signal":
		st := step.NewSignalStep(s.Title, s.Signal.Default, timeout, s.ExitOnFailure)
		st.Signal = toStepOverrideable(s.Signal)
		st.Timeout = toStepOverrideable(s.Timeout)
		return st, nil

	case "dependencies":
		return step.NewDependenciesStep(s.Title), nil

	default:
		return nil, fmt.Errorf("unknown step type: %q", s.Type)
	}
}

func toStepOverrideable(o overrideableV0[string]) step.Overrideable[string] {
	return step.Overrideable[string]{
		Default: o.Default,
		OSArch:  o.OSArch,
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

func parseTimeout(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	return time.ParseDuration(s)
}

package arrow

import (
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/domain"
	domainStep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/ruleset/aerrors"
)

// NoDependenciesStepRule rejects manifests that declare type: dependencies in
// lifecycle steps. The runtime always injects a synthetic dep-resolve step at
// install time — declaring it manually is an error.
type NoDependenciesStepRule struct{}

func (NoDependenciesStepRule) Name() string { return "no_dependencies_step" }

func (NoDependenciesStepRule) Validate(m *domain.Arrow) aerrors.RuleErrors {
	var errs aerrors.RuleErrors
	for os, target := range m.Targets {
		errs = append(errs, checkNoDepsSteps(string(os), target)...)
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

func checkNoDepsSteps(key string, target domain.Target) aerrors.RuleErrors {
	var errs aerrors.RuleErrors
	lcGroups := []struct {
		name  string
		steps domainStep.StepList
	}{
		{"lifecycle.install", target.Lifecycle.Install},
		{"lifecycle.update", target.Lifecycle.Update},
		{"lifecycle.execute", target.Lifecycle.Execute},
		{"lifecycle.stop", target.Lifecycle.Stop},
		{"lifecycle.uninstall", target.Lifecycle.Uninstall},
	}
	for _, lc := range lcGroups {
		for i, s := range lc.steps {
			if s.Type() == domainStep.StepTypeDependencies {
				errs = append(errs, aerrors.RuleError{
					Field:   fmt.Sprintf("targets[%s].%s[%d]", key, lc.name, i),
					Rule:    "no_dependencies_step",
					Message: "type: dependencies must not appear in manifests; the runtime injects it automatically",
				})
			}
		}
	}
	return errs
}

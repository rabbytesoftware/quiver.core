package arrow

import (
	"fmt"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/ruleset/aerrors"
)

type PortableNameRule struct{}

func (PortableNameRule) Name() string { return "portable_name" }

func (PortableNameRule) Validate(
	m *domain.Arrow,
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors
	for os, target := range m.Targets {
		errs = append(errs, checkPortableNames(string(os), target)...)
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

func checkPortableNames(
	key string,
	target domain.Target,
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors

	lcGroups := []struct {
		name  string
		steps step.StepList
	}{
		{"lifecycle.install", target.Lifecycle.Install},
		{"lifecycle.update", target.Lifecycle.Update},
		{"lifecycle.execute", target.Lifecycle.Execute},
		{"lifecycle.stop", target.Lifecycle.Stop},
		{"lifecycle.uninstall", target.Lifecycle.Uninstall},
		{"lifecycle.preinstalled", target.Lifecycle.Preinstalled},
	}
	for _, lc := range lcGroups {
		errs = append(errs, checkStepListPortableNames(key, lc.name, lc.steps)...)
	}
	for methodName, m := range target.Methods {
		field := fmt.Sprintf("methods[%s].steps", methodName)
		errs = append(errs, checkStepListPortableNames(key, field, m.Steps)...)
	}

	return errs
}

func checkStepListPortableNames(
	key string,
	prefix string,
	steps step.StepList,
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors
	for i, s := range steps {
		portable, ok := s.(step.PortableStep)
		if !ok || portable.Name == "" || validPortableName(portable.Name) {
			continue
		}
		errs = append(errs, aerrors.RuleError{
			Field: fmt.Sprintf("targets[%s].%s[%d].name", key, prefix, i),
			Rule:  "invalid_name",
			Message: fmt.Sprintf(
				"name %q must match ^[A-Za-z0-9][A-Za-z0-9._+-]{0,63}$, must not end in a dot, and must not be a Windows device name",
				portable.Name,
			),
		})
	}
	return errs
}

func validPortableName(
	name string,
) bool {
	return exposeNameRe.MatchString(name) &&
		!strings.HasSuffix(name, ".") &&
		!domain.IsWindowsReservedName(name)
}

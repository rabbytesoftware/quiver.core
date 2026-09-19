package arrow

import (
	"fmt"
	"path"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/ruleset/aerrors"
)

type OverrideableCoverageRule struct{}

func (OverrideableCoverageRule) Name() string { return "overrideable_coverage" }

func (OverrideableCoverageRule) Validate(
	_ *domain.Arrow,
	precompiled map[string]models.PrecompiledTarget,
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors
	for targetKey, target := range precompiled {
		if strings.HasPrefix(targetKey, "_") {
			continue
		}
		errs = append(errs, checkTargetCoverage(targetKey, target)...)
	}
	return errs
}

// targetScope returns the OSes a target key applies to.
//
// A target keyed "linux/*" never runs anywhere but linux, so requiring its
// overrides to cover windows rejects a manifest that is complete for everything
// the target actually supports.
//
// A key that names no OS is not an OS pattern at all. It yields every OS rather
// than none: narrowing to an empty set would skip this rule for that target
// instead of enforcing it.
func targetScope(targetKey string) []domain.OS {
	all := domain.AllOS()

	scope := make([]domain.OS, 0, len(all))
	for _, os := range all {
		if matchesTargetKey(targetKey, os) {
			scope = append(scope, os)
		}
	}

	if len(scope) == 0 {
		return all
	}

	return scope
}

func matchesTargetKey(
	key string,
	os domain.OS,
) bool {
	if key == "*" {
		return true
	}

	matched, err := path.Match(key, string(os))

	return err == nil && matched
}

func coversScope(
	osArch map[string]string,
	scope []domain.OS,
) (bool, domain.OS) {
	if _, ok := osArch["*"]; ok {
		return true, ""
	}
	for _, os := range scope {
		if !osArchCoversOS(osArch, os) {
			return false, os
		}
	}
	return true, ""
}

func osArchCoversOS(
	osArch map[string]string,
	os domain.OS,
) bool {
	for k := range osArch {
		matched, err := path.Match(k, string(os))
		if err == nil && matched {
			return true
		}
	}
	return false
}

func checkStringCoverage(
	ov step.Overrideable[string],
	field string,
	scope []domain.OS,
) aerrors.RuleErrors {
	if ov.Default != "" {
		return nil
	}
	ok, missing := coversScope(ov.OSArch, scope)
	if ok {
		return nil
	}
	return aerrors.RuleErrors{{
		Field:   field,
		Rule:    "insufficient_coverage",
		Message: fmt.Sprintf("%s has no default and is missing coverage for OS %q", field, missing),
	}}
}

func checkTargetCoverage(
	targetKey string,
	target models.PrecompiledTarget,
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors

	scope := targetScope(targetKey)

	for exportKey, ov := range target.Exports {
		field := fmt.Sprintf("targets[%s].exports.%s", targetKey, exportKey)
		errs = append(errs, checkStringCoverage(ov, field, scope)...)
	}

	phases := []struct {
		name  string
		steps step.StepList
	}{
		{"install", target.Lifecycle.Install},
		{"update", target.Lifecycle.Update},
		{"execute", target.Lifecycle.Execute},
		{"stop", target.Lifecycle.Stop},
		{"uninstall", target.Lifecycle.Uninstall},
	}
	for _, phase := range phases {
		errs = append(errs, checkStepListCoverage(targetKey, phase.name, phase.steps, scope)...)
	}

	for methodName, method := range target.Methods {
		errs = append(errs, checkStepListCoverage(targetKey, "methods."+methodName, method.Steps, scope)...)
	}

	return errs
}

func isPopulated(ov step.Overrideable[string]) bool {
	return ov.Default != "" || len(ov.OSArch) > 0
}

func checkStepListCoverage(
	targetKey string,
	phase string,
	steps step.StepList,
	scope []domain.OS,
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors
	for i, s := range steps {
		stepField := func(fieldName string) string {
			return fmt.Sprintf("targets[%s].lifecycle.%s[%d].%s", targetKey, phase, i, fieldName)
		}
		switch v := s.(type) {
		case step.RunStep:
			errs = append(errs, checkStringCoverage(v.Command, stepField("command"), scope)...)
			if isPopulated(v.Timeout) {
				errs = append(errs, checkStringCoverage(v.Timeout, stepField("timeout"), scope)...)
			}
		case step.FetchStep:
			errs = append(errs, checkStringCoverage(v.URL, stepField("url"), scope)...)
			errs = append(errs, checkStringCoverage(v.To, stepField("to"), scope)...)
			if isPopulated(v.Checksum) {
				errs = append(errs, checkStringCoverage(v.Checksum, stepField("checksum"), scope)...)
			}
			if isPopulated(v.Timeout) {
				errs = append(errs, checkStringCoverage(v.Timeout, stepField("timeout"), scope)...)
			}
		case step.SignalStep:
			if isPopulated(v.Timeout) {
				errs = append(errs, checkStringCoverage(v.Timeout, stepField("timeout"), scope)...)
			}
		}
	}
	return errs
}

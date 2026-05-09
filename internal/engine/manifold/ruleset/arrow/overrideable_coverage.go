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

func coversAllOS(
	osArch map[string]string,
) (bool, domain.OS) {
	if _, ok := osArch["*"]; ok {
		return true, ""
	}
	for _, os := range domain.AllOS() {
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
) aerrors.RuleErrors {
	if ov.Default != "" {
		return nil
	}
	ok, missing := coversAllOS(ov.OSArch)
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

	for exportKey, ov := range target.Exports {
		field := fmt.Sprintf("targets[%s].exports.%s", targetKey, exportKey)
		errs = append(errs, checkStringCoverage(ov, field)...)
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
		errs = append(errs, checkStepListCoverage(targetKey, phase.name, phase.steps)...)
	}

	for methodName, method := range target.Methods {
		errs = append(errs, checkStepListCoverage(targetKey, "methods."+methodName, method.Steps)...)
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
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors
	for i, s := range steps {
		stepField := func(fieldName string) string {
			return fmt.Sprintf("targets[%s].lifecycle.%s[%d].%s", targetKey, phase, i, fieldName)
		}
		switch v := s.(type) {
		case step.RunStep:
			errs = append(errs, checkStringCoverage(v.Command, stepField("command"))...)
			if isPopulated(v.Timeout) {
				errs = append(errs, checkStringCoverage(v.Timeout, stepField("timeout"))...)
			}
		case step.FetchStep:
			errs = append(errs, checkStringCoverage(v.URL, stepField("url"))...)
			errs = append(errs, checkStringCoverage(v.To, stepField("to"))...)
			if isPopulated(v.Checksum) {
				errs = append(errs, checkStringCoverage(v.Checksum, stepField("checksum"))...)
			}
			if isPopulated(v.Timeout) {
				errs = append(errs, checkStringCoverage(v.Timeout, stepField("timeout"))...)
			}
		case step.SignalStep:
			if isPopulated(v.Timeout) {
				errs = append(errs, checkStringCoverage(v.Timeout, stepField("timeout"))...)
			}
		}
	}
	return errs
}

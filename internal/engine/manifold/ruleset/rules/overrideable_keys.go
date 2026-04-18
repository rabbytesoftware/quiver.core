package rules

import (
	"fmt"
	"strings"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/models"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/ruleset/aerrors"
)

type OverrideableKeysRule struct{}

func (OverrideableKeysRule) Name() string { return "overrideable_keys" }

func (OverrideableKeysRule) Validate(
	_ *domain.ArrowManifest,
	precompiled map[string]models.PrecompiledTarget,
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors
	for targetKey, target := range precompiled {
		errs = append(errs, checkTargetKeys(targetKey, target)...)
	}
	return errs
}

func isValidOverrideableKey(
	k string,
) bool {
	return k == "*" || strings.Contains(k, "/")
}

func invalidKeyError(
	field string,
	k string,
) aerrors.RuleError {
	return aerrors.RuleError{
		Field:   field,
		Rule:    "invalid_overrideable_key",
		Message: fmt.Sprintf("key %q must be \"*\" or contain \"/\"", k),
	}
}

func checkStringKeys(
	osArch map[string]string,
	field string,
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors
	for k := range osArch {
		if !isValidOverrideableKey(k) {
			errs = append(errs, invalidKeyError(field, k))
		}
	}
	return errs
}

func checkBoolKeys(
	osArch map[string]bool,
	field string,
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors
	for k := range osArch {
		if !isValidOverrideableKey(k) {
			errs = append(errs, invalidKeyError(field, k))
		}
	}
	return errs
}

func checkSignalKindKeys(
	osArch map[string]step.SignalKind,
	field string,
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors
	for k := range osArch {
		if !isValidOverrideableKey(k) {
			errs = append(errs, invalidKeyError(field, k))
		}
	}
	return errs
}

func checkTargetKeys(
	targetKey string,
	target models.PrecompiledTarget,
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors

	for exportKey, ov := range target.Exports {
		field := fmt.Sprintf("targets[%s].exports.%s", targetKey, exportKey)
		errs = append(errs, checkStringKeys(ov.OSArch, field)...)
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
		errs = append(errs, checkStepListKeys(targetKey, phase.name, phase.steps)...)
	}

	for methodName, method := range target.Methods {
		errs = append(errs, checkStepListKeys(targetKey, "methods."+methodName, method.Steps)...)
	}

	return errs
}

func checkStepListKeys(
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
			errs = append(errs, checkStringKeys(v.Command.OSArch, stepField("command"))...)
			errs = append(errs, checkStringKeys(v.Timeout.OSArch, stepField("timeout"))...)
			errs = append(errs, checkBoolKeys(v.Elevated.OSArch, stepField("elevated"))...)
		case step.FetchStep:
			errs = append(errs, checkStringKeys(v.URL.OSArch, stepField("url"))...)
			errs = append(errs, checkStringKeys(v.To.OSArch, stepField("to"))...)
			errs = append(errs, checkStringKeys(v.Checksum.OSArch, stepField("checksum"))...)
			errs = append(errs, checkStringKeys(v.Timeout.OSArch, stepField("timeout"))...)
		case step.SignalStep:
			errs = append(errs, checkSignalKindKeys(v.Signal.OSArch, stepField("signal"))...)
			errs = append(errs, checkStringKeys(v.Timeout.OSArch, stepField("timeout"))...)
		}
	}
	return errs
}

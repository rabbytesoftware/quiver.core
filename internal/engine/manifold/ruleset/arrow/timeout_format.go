package arrow

import (
	"fmt"
	"regexp"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/ruleset/aerrors"
)

var timeoutRe = regexp.MustCompile(`^\d+[sm]$`)

type TimeoutFormatRule struct{}

func (TimeoutFormatRule) Name() string { return "timeout_format" }

func (TimeoutFormatRule) Validate(
	m *domain.Arrow,
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors
	for os, target := range m.Targets {
		errs = append(errs, checkTimeoutFormat(string(os), target)...)
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

func checkTimeoutFormat(
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
	}
	for _, lc := range lcGroups {
		errs = append(errs, checkStepListTimeouts(key, lc.name, lc.steps)...)
	}
	for methodName, m := range target.Methods {
		field := fmt.Sprintf("methods[%s].steps", methodName)
		errs = append(errs, checkStepListTimeouts(key, field, m.Steps)...)
	}

	return errs
}

func checkStepListTimeouts(
	key string,
	prefix string,
	steps step.StepList,
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors
	for i, s := range steps {
		timeout := extractStepTimeout(s)
		if timeout == "" {
			continue
		}
		if timeoutRe.MatchString(timeout) {
			continue
		}
		errs = append(errs, aerrors.RuleError{
			Field:   fmt.Sprintf("targets[%s].%s[%d].timeout", key, prefix, i),
			Rule:    "invalid_timeout",
			Message: fmt.Sprintf("timeout %q must match \\d+[sm] (e.g. 30s, 5m)", timeout),
		})
	}
	return errs
}

func extractStepTimeout(
	s step.Step,
) string {
	switch v := s.(type) {
	case step.RunStep:
		return v.Timeout.Default
	case step.FetchStep:
		return v.Timeout.Default
	case step.SignalStep:
		return v.Timeout.Default
	}
	return ""
}

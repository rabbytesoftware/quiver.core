package arrow

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/ruleset/aerrors"
)

var varTokenRe = regexp.MustCompile(`\$\{([^}]+)\}`)

type VariableRefsRule struct{}

func (VariableRefsRule) Name() string { return "variable_refs" }

func (VariableRefsRule) Validate(
	m *domain.Arrow,
) aerrors.RuleErrors {
	known := buildKnownVars(m)
	var errs aerrors.RuleErrors
	for os, t := range m.Targets {
		errs = append(errs, checkCompiledTargetVariableRefs(string(os), t, known)...)
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

func buildKnownVars(
	m *domain.Arrow,
) map[string]bool {
	known := map[string]bool{
		"WORKDIR":         true,
		"INSTALL_PATH":    true,
		"ARROW_NAMESPACE": true,
		"PLATFORM":        true,
		"REF":             true,
	}
	for _, v := range m.Variables {
		known[v.Name] = true
	}
	for _, p := range m.Netbridge {
		known[p.Name] = true
	}
	return known
}

func checkCompiledTargetVariableRefs(
	key string,
	t domain.Target,
	known map[string]bool,
) aerrors.RuleErrors {
	prefix := fmt.Sprintf("targets[%s]", key)
	var errs aerrors.RuleErrors

	allSteps := []step.StepList{
		t.Lifecycle.Install,
		t.Lifecycle.Update,
		t.Lifecycle.Execute,
		t.Lifecycle.Stop,
		t.Lifecycle.Uninstall,
	}
	for _, steps := range allSteps {
		errs = append(errs, checkStepListVars(prefix, steps, known)...)
	}
	for methodName, method := range t.Methods {
		methodPrefix := fmt.Sprintf("targets[%s].methods[%s]", key, methodName)
		errs = append(errs, checkStepListVars(methodPrefix, method.Steps, known)...)
	}
	return errs
}

func checkStepListVars(
	prefix string,
	steps step.StepList,
	known map[string]bool,
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors
	for _, s := range steps {
		errs = append(errs, checkCompiledStepVars(prefix, s, known)...)
	}
	return errs
}

func overrideableValues(
	ov step.Overrideable[string],
) []string {
	vals := make([]string, 0, 1+len(ov.OSArch))
	vals = append(vals, ov.Default)
	for _, v := range ov.OSArch {
		vals = append(vals, v)
	}
	return vals
}

func checkCompiledStepVars(
	prefix string,
	s step.Step,
	known map[string]bool,
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors
	switch rs := s.(type) {
	case step.RunStep:
		errs = append(errs, checkVarTokens(prefix, "command", overrideableValues(rs.Command), known)...)
	case step.FetchStep:
		errs = append(errs, checkVarTokens(prefix, "url", overrideableValues(rs.URL), known)...)
		errs = append(errs, checkVarTokens(prefix, "to", overrideableValues(rs.To), known)...)
	}
	return errs
}

func checkVarTokens(
	prefix string,
	field string,
	values []string,
	known map[string]bool,
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors
	for _, v := range values {
		errs = append(errs, checkSingleValueTokens(prefix, field, v, known)...)
	}
	return errs
}

func checkSingleValueTokens(
	prefix string,
	field string,
	v string,
	known map[string]bool,
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors
	for _, token := range extractVarTokens(v) {
		if strings.Contains(token, ".") || strings.Contains(token, ":") {
			continue
		}
		if !known[token] {
			errs = append(errs, aerrors.RuleError{
				Field:   fmt.Sprintf("%s.%s", prefix, field),
				Rule:    "unresolved_variable",
				Message: fmt.Sprintf("unknown variable ${%s}", token),
			})
		}
	}
	return errs
}

func extractVarTokens(
	s string,
) []string {
	matches := varTokenRe.FindAllStringSubmatch(s, -1)
	tokens := make([]string, 0, len(matches))
	for _, m := range matches {
		tokens = append(tokens, m[1])
	}
	return tokens
}

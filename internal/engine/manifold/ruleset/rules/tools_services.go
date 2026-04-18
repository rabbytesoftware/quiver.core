package rules

import (
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/ruleset/aerrors"
)

type ToolsServicesRule struct{}

func (ToolsServicesRule) Name() string { return "tools_services" }

func (ToolsServicesRule) Validate(
	m *domain.ArrowManifest,
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors
	for os, t := range m.Targets {
		errs = append(errs, checkTargetToolServiceOverlap(string(os), t)...)
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

func checkTargetToolServiceOverlap(
	key string,
	t domain.Target,
) aerrors.RuleErrors {
	toolNS := make(map[string]bool, len(t.Tools))
	for _, n := range t.Tools {
		toolNS[string(n.Namespace.BareNamespace())] = true
	}
	var errs aerrors.RuleErrors
	for _, n := range t.Services {
		bare := string(n.Namespace.BareNamespace())
		if !toolNS[bare] {
			continue
		}
		errs = append(errs, aerrors.RuleError{
			Field:   fmt.Sprintf("targets[%s]", key),
			Rule:    "tools_services_overlap",
			Message: fmt.Sprintf("namespace %q appears in both tools and services", bare),
		})
	}
	return errs
}

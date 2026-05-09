package arrow

import (
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/ruleset/aerrors"
)

type ServicePackageRule struct{}

func (ServicePackageRule) Name() string { return "service_package" }

func (ServicePackageRule) Validate(
	m *domain.Arrow,
) aerrors.RuleErrors {
	hasService := false
	hasPackage := false
	for _, target := range m.Targets {
		if target.Lifecycle.Execute != nil {
			hasService = true
		} else {
			hasPackage = true
		}
	}
	if hasService && hasPackage {
		return aerrors.RuleErrors{{
			Field:   "targets",
			Rule:    "mixed_kind",
			Message: "manifest mixes service targets (with execute) and package targets (without execute)",
		}}
	}
	return nil
}

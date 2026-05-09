package arrow

import (
	"fmt"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/ruleset/aerrors"
)

type NetbridgeRule struct{}

func (NetbridgeRule) Name() string { return "netbridge" }

func (NetbridgeRule) Validate(
	m *domain.Arrow,
	_ map[string]models.PrecompiledTarget,
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors
	names := make([]string, 0, len(m.Netbridge))

	for i, p := range m.Netbridge {
		if err := p.Validate(); err != nil {
			errs = append(errs, aerrors.RuleError{
				Field:   fmt.Sprintf("netbridge[%d]", i),
				Rule:    "invalid_port",
				Message: err.Error(),
			})
			continue
		}
		names = append(names, p.Name)
	}

	errs = append(errs, checkDuplicates(names, "netbridge", "duplicate_name")...)
	if len(errs) == 0 {
		return nil
	}
	return errs
}

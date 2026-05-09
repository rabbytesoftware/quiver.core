package arrow

import (
	"fmt"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/ruleset/aerrors"
)

type ExportStaticRule struct{}

func (ExportStaticRule) Name() string { return "export_static" }

func (ExportStaticRule) Validate(
	m *domain.Arrow,
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors
	for os, t := range m.Targets {
		errs = append(errs, checkTargetExportStaticValues(string(os), t)...)
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

func checkTargetExportStaticValues(
	key string,
	t domain.Target,
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors
	for exportKey, v := range t.Exports {
		if !containsVar(v) {
			continue
		}
		errs = append(errs, aerrors.RuleError{
			Field:   fmt.Sprintf("targets[%s].exports.%s", key, exportKey),
			Rule:    "export_var_interpolation",
			Message: fmt.Sprintf("export value %q must not contain variable interpolations", v),
		})
	}
	return errs
}

func containsVar(
	s string,
) bool {
	return strings.Contains(s, "${")
}

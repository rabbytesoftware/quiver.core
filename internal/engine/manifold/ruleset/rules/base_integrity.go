package rules

import (
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/models"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/ruleset/aerrors"
)

type BaseIntegrityRule struct{}

func (BaseIntegrityRule) Name() string { return "base_integrity" }

func (BaseIntegrityRule) Validate(
	_ *domain.Arrow,
	precompiled map[string]models.PrecompiledTarget,
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors
	for key := range precompiled {
		errs = append(errs, checkBaseChain(key, precompiled)...)
	}
	return errs
}

func checkBaseChain(
	key string,
	precompiled map[string]models.PrecompiledTarget,
) aerrors.RuleErrors {
	seen := map[string]bool{}
	current := key

	for range len(precompiled) + 1 {
		target := precompiled[current]
		base := target.Base
		if base == "" {
			return nil
		}

		if seen[base] {
			return aerrors.RuleErrors{{
				Field:   fmt.Sprintf("targets[%s].base", key),
				Rule:    "cyclic_base",
				Message: fmt.Sprintf("cycle detected: %q appears more than once in the base chain", base),
			}}
		}

		if _, ok := precompiled[base]; !ok {
			return aerrors.RuleErrors{{
				Field:   fmt.Sprintf("targets[%s].base", key),
				Rule:    "missing_base",
				Message: fmt.Sprintf("base %q does not exist in targets", base),
			}}
		}

		seen[current] = true
		current = base
	}

	return aerrors.RuleErrors{{
		Field:   fmt.Sprintf("targets[%s].base", key),
		Rule:    "cyclic_base",
		Message: fmt.Sprintf("cycle detected in base chain starting from %q", key),
	}}
}

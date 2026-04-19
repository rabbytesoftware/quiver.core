package ruleset

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/models"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/ruleset/rules"
)

// Ruleset runs all business-rule validations against an ArrowManifest.
type Ruleset interface {
	ValidatePrecompile(
		manifest *domain.Arrow,
		precompiled map[string]models.PrecompiledTarget,
	) error

	ValidateCompiled(
		manifest *domain.Arrow,
	) error
}

// New returns a Ruleset with all built-in rules registered.
func New() Ruleset {
	return &ruleset{}
}

type ruleset struct{}

func (r *ruleset) ValidatePrecompile(
	manifest *domain.Arrow,
	precompiled map[string]models.PrecompiledTarget,
) error {
	var errs RuleErrors

	if err := rules.RunPrecompile(context.Background(), manifest, precompiled); err != nil {
		if ae, ok := err.(RuleErrors); ok {
			errs = append(errs, ae...)
		}
	}

	if len(errs) == 0 {
		return nil
	}
	return errs
}

func (r *ruleset) ValidateCompiled(
	manifest *domain.Arrow,
) error {
	var errs RuleErrors

	if err := rules.RunCompiled(context.Background(), manifest); err != nil {
		if ae, ok := err.(RuleErrors); ok {
			errs = append(errs, ae...)
		}
	}

	if len(manifest.Targets) == 0 {
		errs = append(errs, RuleError{
			Field:   "targets",
			Rule:    "no_supported_platform",
			Message: "manifest has no target that matches any supported OS",
		})
	}

	if len(errs) == 0 {
		return nil
	}
	return errs
}

// ValidateQuiver applies all business rules to a domain.QuiverManifest.
func ValidateQuiver(
	manifest *domain.QuiverManifest,
) error {
	return nil
}

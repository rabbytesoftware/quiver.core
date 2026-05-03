package ruleset

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/models"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/ruleset/arrow"
	quiverrules "github.com/rabbytesoftware/quiver/internal/engine/manifold/ruleset/quiver"
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

	ValidateQuiver(manifest *domain.QuiverManifest) error
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

	if err := arrow.RunPrecompile(context.Background(), manifest, precompiled); err != nil {
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

	if err := arrow.RunCompiled(context.Background(), manifest); err != nil {
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

func (r *ruleset) ValidateQuiver(manifest *domain.QuiverManifest) error {
	if manifest.Name == "" {
		return RuleErrors{RuleError{
			Field:   "name",
			Rule:    "required",
			Message: "name is required",
		}}
	}
	if manifest.Description == "" {
		return RuleErrors{RuleError{
			Field:   "description",
			Rule:    "required",
			Message: "description is required",
		}}
	}
	return quiverrules.CheckDuplicateNamespaces(manifest.Arrows)
}

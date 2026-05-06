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

	ValidateQuiver(quiver *domain.Quiver) error

	ValidateQuiverEntries(entries []domain.QuiverArrowEntry) error
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

func (r *ruleset) ValidateQuiver(quiver *domain.Quiver) error {
	if quiver.Meta.Name == "" {
		return RuleErrors{RuleError{
			Field:   "name",
			Rule:    "required",
			Message: "name is required",
		}}
	}
	if quiver.Meta.Description == "" {
		return RuleErrors{RuleError{
			Field:   "description",
			Rule:    "required",
			Message: "description is required",
		}}
	}
	if len(quiver.Arrows) == 0 {
		return RuleErrors{RuleError{
			Field:   "arrows",
			Rule:    "required",
			Message: "arrows list must not be empty",
		}}
	}
	return quiverrules.CheckDuplicateNamespaces(quiver.Arrows)
}

func (r *ruleset) ValidateQuiverEntries(entries []domain.QuiverArrowEntry) error {
	return quiverrules.CheckArrowEntries(entries)
}

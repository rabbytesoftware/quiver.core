package arrow

import (
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/ruleset/aerrors"
)

// PrecompileRule validates the raw precompiled shape before OS compilation.
type PrecompileRule interface {
	Name() string
	Validate(manifest *domain.Arrow, precompiled map[string]models.PrecompiledTarget) aerrors.RuleErrors
}

// CompiledRule validates the compiled manifest after OS-specific compilation.
type CompiledRule interface {
	Name() string
	Validate(manifest *domain.Arrow) aerrors.RuleErrors
}

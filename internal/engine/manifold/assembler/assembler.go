package assembler

import (
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/assembler/step"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/models"
)

// Assembler validates and builds domain aggregates from raw manifest models.
type Assembler struct {
	stepHandlers map[string]step.Handler
}

// New returns an Assembler with all built-in step handlers registered.
func New() *Assembler {
	a := &Assembler{
		stepHandlers: make(map[string]step.Handler),
	}
	a.registerStepHandler("run", step.NewRunHandler())
	a.registerStepHandler("fetch", step.NewFetchHandler())
	a.registerStepHandler("signal", step.NewSignalHandler())
	a.registerStepHandler("dependencies", step.NewDependenciesHandler())

	return a
}

func (a *Assembler) registerStepHandler(
	typeName string,
	handler step.Handler,
) {
	a.stepHandlers[typeName] = handler
}

// AssembleArrow applies all business rules to a RawArrow for the given OS
// and returns a fully validated domain.ArrowManifest.
func (a *Assembler) AssembleArrow(
	raw *models.RawArrow,
	os string,
) (*domain.ArrowManifest, error) {
	if err := checkOSCompatibility(os, raw.Requirements.OS); err != nil {
		return nil, err
	}

	resolvedLC, resolvedMethods, err := a.resolveAllOverrides(
		raw.Lifecycle,
		raw.Methods,
		os,
		raw.Requirements.OS,
	)
	if err != nil {
		return nil, err
	}

	if err := a.validateArrow(raw, resolvedLC, resolvedMethods); err != nil {
		return nil, err
	}

	return a.buildArrow(raw, resolvedLC, resolvedMethods)
}

// AssembleQuiver converts a RawQuiver into a domain.QuiverManifest.
func (a *Assembler) AssembleQuiver(
	raw *models.RawQuiver,
) (*domain.QuiverManifest, error) {
	arrows := make([]domain.Namespace, 0, len(raw.Arrows))
	for _, ar := range raw.Arrows {
		arrows = append(arrows, domain.Namespace(ar))
	}

	return &domain.QuiverManifest{
		Name:        raw.Name,
		Description: raw.Description,
		URL:         raw.URL,
		Maintainers: raw.Maintainers,
		Tags:        raw.Tags,
		Media:       raw.Media,
		Arrows:      arrows,
	}, nil
}

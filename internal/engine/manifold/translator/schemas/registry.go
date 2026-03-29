package schemas

import (
	"fmt"
	"strings"

	"github.com/rabbytesoftware/quiver/internal/engine/manifold/models"
)

// Registry holds the registered Mapper implementations keyed by manifest key
// (e.g. "arrow@v0", "quiver@v0").
type Registry struct {
	arrowMappers  map[string]Mapper[models.RawArrow]
	quiverMappers map[string]Mapper[models.RawQuiver]
}

// NewRegistry returns a Registry pre-loaded with all registered mappers.
func NewRegistry() *Registry {
	r := &Registry{
		arrowMappers:  make(map[string]Mapper[models.RawArrow]),
		quiverMappers: make(map[string]Mapper[models.RawQuiver]),
	}
	r.register()

	return r
}

// GetArrowMapper returns the Mapper for the given arrow manifest key.
func (r *Registry) GetArrowMapper(
	manifestKey string,
) (Mapper[models.RawArrow], error) {
	mapper, ok := r.arrowMappers[manifestKey]
	if !ok {
		return nil, fmt.Errorf("unsupported arrow manifest: %s", manifestKey)
	}

	return mapper, nil
}

// GetQuiverMapper returns the Mapper for the given quiver manifest key.
func (r *Registry) GetQuiverMapper(
	manifestKey string,
) (Mapper[models.RawQuiver], error) {
	mapper, ok := r.quiverMappers[manifestKey]
	if !ok {
		return nil, fmt.Errorf("unsupported quiver manifest: %s", manifestKey)
	}

	return mapper, nil
}

// GetSchema returns the JSON schema bytes for the given manifest key.
func (r *Registry) GetSchema(
	manifestKey string,
) ([]byte, error) {
	if strings.HasPrefix(manifestKey, "arrow@") {
		if mapper, ok := r.arrowMappers[manifestKey]; ok {
			return mapper.GetSchema()
		}
	}

	if strings.HasPrefix(manifestKey, "quiver@") {
		if mapper, ok := r.quiverMappers[manifestKey]; ok {
			return mapper.GetSchema()
		}
	}

	return nil, fmt.Errorf("no schema found for: %s", manifestKey)
}

// IsSupported reports whether the given manifest key has a registered mapper.
func (r *Registry) IsSupported(
	manifestKey string,
) bool {
	if strings.HasPrefix(manifestKey, "arrow@") {
		_, ok := r.arrowMappers[manifestKey]
		return ok
	}

	if strings.HasPrefix(manifestKey, "quiver@") {
		_, ok := r.quiverMappers[manifestKey]
		return ok
	}

	return false
}

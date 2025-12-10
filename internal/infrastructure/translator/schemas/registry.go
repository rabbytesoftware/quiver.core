package schemas

import (
	"fmt"
	"strings"

	"github.com/rabbytesoftware/quiver/internal/models/arrow"
	"github.com/rabbytesoftware/quiver/internal/models/quiver"
)

type Registry struct {
	arrowMappers  map[string]Mapper[arrow.Arrow]
	quiverMappers map[string]Mapper[quiver.Quiver]
}

func NewRegistry() *Registry {
	r := &Registry{
		arrowMappers:  make(map[string]Mapper[arrow.Arrow]),
		quiverMappers: make(map[string]Mapper[quiver.Quiver]),
	}
	r.register()
	return r
}

func (r *Registry) GetArrowMapper(manifestKey string) (Mapper[arrow.Arrow], error) {
	mapper, ok := r.arrowMappers[manifestKey]
	if !ok {
		return nil, fmt.Errorf("unsupported arrow manifest: %s", manifestKey)
	}
	return mapper, nil
}

func (r *Registry) GetQuiverMapper(manifestKey string) (Mapper[quiver.Quiver], error) {
	mapper, ok := r.quiverMappers[manifestKey]
	if !ok {
		return nil, fmt.Errorf("unsupported quiver manifest: %s", manifestKey)
	}
	return mapper, nil
}

func (r *Registry) GetSchema(manifestKey string) ([]byte, error) {
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

func (r *Registry) IsSupported(manifestKey string) bool {
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

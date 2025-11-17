package schemas

import (
	"fmt"
	"strings"

	arrowv1 "github.com/rabbytesoftware/quiver/internal/infrastructure/translator/schemas/arrow/v1"
	quiverv1 "github.com/rabbytesoftware/quiver/internal/infrastructure/translator/schemas/quiver/v1"
	"github.com/rabbytesoftware/quiver/internal/models/arrow"
	"github.com/rabbytesoftware/quiver/internal/models/quiver"
)

type Registry interface {
	GetArrowMapper(manifestKey string) (Mapper[arrow.Arrow], error)
	GetQuiverMapper(manifestKey string) (Mapper[quiver.Quiver], error)
	GetSchema(manifestKey string) ([]byte, error)
	IsSupported(manifestKey string) bool
}

type RegistryImplementation struct {
	arrowMappers  map[string]Mapper[arrow.Arrow]
	quiverMappers map[string]Mapper[quiver.Quiver]
}

func NewRegistry() Registry {
	r := &RegistryImplementation{
		arrowMappers:  make(map[string]Mapper[arrow.Arrow]),
		quiverMappers: make(map[string]Mapper[quiver.Quiver]),
	}
	r.register()
	return r
}

func (r *RegistryImplementation) register() {
	r.arrowMappers["arrow@v1"] = arrowv1.NewMapper()

	r.quiverMappers["quiver@v1"] = quiverv1.NewMapper()
}

func (r *RegistryImplementation) GetArrowMapper(manifestKey string) (Mapper[arrow.Arrow], error) {
	mapper, ok := r.arrowMappers[manifestKey]
	if !ok {
		return nil, fmt.Errorf("unsupported arrow manifest: %s", manifestKey)
	}
	return mapper, nil
}

func (r *RegistryImplementation) GetQuiverMapper(manifestKey string) (Mapper[quiver.Quiver], error) {
	mapper, ok := r.quiverMappers[manifestKey]
	if !ok {
		return nil, fmt.Errorf("unsupported quiver manifest: %s", manifestKey)
	}
	return mapper, nil
}

func (r *RegistryImplementation) GetSchema(manifestKey string) ([]byte, error) {
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

func (r *RegistryImplementation) IsSupported(manifestKey string) bool {
	// Check arrow mappers
	if strings.HasPrefix(manifestKey, "arrow@") {
		_, ok := r.arrowMappers[manifestKey]
		return ok
	}

	// Check quiver mappers
	if strings.HasPrefix(manifestKey, "quiver@") {
		_, ok := r.quiverMappers[manifestKey]
		return ok
	}

	return false
}

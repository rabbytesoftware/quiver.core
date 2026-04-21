package arrow

import (
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/models"
	arrowv0 "github.com/rabbytesoftware/quiver/internal/engine/manifold/translator/arrow/v0"
)

// Module is the interface that each versioned Arrow translator must satisfy.
type Module interface {
	Schema() []byte

	Parse(
		data []byte,
	) (
		*domain.Arrow,
		map[string]models.PrecompiledTarget,
		error,
	)

	Selector() models.Selector
}

type Registry struct {
	versions map[string]Module
}

func New() *Registry {
	return NewRegistry()
}

func NewRegistry() *Registry {
	r := &Registry{versions: make(map[string]Module)}

	r.register("v0", arrowv0.New())

	return r
}

func (r *Registry) register(
	version string,
	m Module,
) {
	r.versions[version] = m
}

func (r *Registry) Get(
	version string,
) (Module, error) {
	m, ok := r.versions[version]

	if !ok {
		return nil, fmt.Errorf("unsupported arrow version: %s", version)
	}

	return m, nil
}

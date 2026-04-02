package arrow

import (
	"fmt"

	arrowv0 "github.com/rabbytesoftware/quiver/internal/engine/manifold/translator/arrow/v0"
)

type Registry struct {
	modules map[string]Module
}

func NewRegistry() *Registry {
	r := &Registry{modules: make(map[string]Module)}

	r.register(arrowv0.Default)

	return r
}

func (r *Registry) register(m Module) {
	r.modules[m.Version()] = m
}

func (r *Registry) Get(version string) (Module, error) {
	m, ok := r.modules[version]
	if !ok {
		return nil, fmt.Errorf("unsupported arrow version: %s", version)
	}
	return m, nil
}

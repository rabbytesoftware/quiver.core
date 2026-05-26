package v0

import (
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/models"
)

type v0 struct{}

func New() *v0 {
	return &v0{}
}

func (m *v0) Version() string {
	return "v0"
}

func (m *v0) Schema() []byte {
	return schemaJSON
}

func (m *v0) Parse(
	data []byte,
) (*domain.Arrow, map[string]models.PrecompiledTarget, error) {
	return Map(data)
}

func (m *v0) Selector() models.Selector {
	return &selector{}
}

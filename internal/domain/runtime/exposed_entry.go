package runtime

import "github.com/rabbytesoftware/quiver.core/internal/domain"

type ExposedEntry struct {
	Kind     domain.ExposeKind `yaml:"kind"     json:"kind"`
	Name     string            `yaml:"name"     json:"name"`
	Target   string            `yaml:"target"   json:"target"`
	Location string            `yaml:"location" json:"location"`
}

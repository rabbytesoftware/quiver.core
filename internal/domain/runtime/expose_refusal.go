package runtime

import "github.com/rabbytesoftware/quiver.core/internal/domain"

type ExposeRefusal struct {
	Kind   domain.ExposeKind `yaml:"kind"   json:"kind"`
	Name   string            `yaml:"name"   json:"name"`
	Reason string            `yaml:"reason" json:"reason"`
}

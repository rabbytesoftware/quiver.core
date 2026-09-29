package runtime

import "github.com/rabbytesoftware/quiver.core/internal/domain"

type ExposeResult struct {
	Entries []ExposedEntry  `yaml:"entries" json:"entries"`
	Refused []ExposeRefusal `yaml:"refused" json:"refused"`
}

type ExposedEntry struct {
	Kind     domain.ExposeKind `yaml:"kind"     json:"kind"`
	Name     string            `yaml:"name"     json:"name"`
	Target   string            `yaml:"target"   json:"target"`
	Location string            `yaml:"location" json:"location"`
}

type ExposeRefusal struct {
	Kind   domain.ExposeKind `yaml:"kind"   json:"kind"`
	Name   string            `yaml:"name"   json:"name"`
	Reason string            `yaml:"reason" json:"reason"`
}

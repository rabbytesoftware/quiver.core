package shelf

import (
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type AppliedEntry struct {
	Kind     domain.ExposeKind `json:"kind"`
	Name     string            `json:"name"`
	Target   string            `json:"target"`
	Location string            `json:"location"`
}

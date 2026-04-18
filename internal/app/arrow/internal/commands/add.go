package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

type AddArrow struct {
	Namespace     domain.Namespace
	Version       domain.ArrowVersion
	DirectInstall bool
}

func (c AddArrow) AggregateID() string {
	return c.Namespace.BareNamespace().String()
}

func (c AddArrow) EventName() string {
	return "arrow.added"
}

func (c AddArrow) ShouldSnapshot() bool {
	return true
}

func (c AddArrow) Validate(current *domain.Arrow) error {
	if current != nil {
		return fmt.Errorf("add arrow: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c AddArrow) EmitEvent(_ *domain.Arrow) domain.Arrow {
	key := c.Namespace.Ref()
	if key == "" {
		key = "latest"
	}

	av := c.Version
	av.DirectInstall = c.DirectInstall
	av.InstalledRef = c.Namespace.Ref()
	if av.InstalledAt.IsZero() {
		av.InstalledAt = time.Now()
	}

	return domain.Arrow{
		Namespace: c.Namespace.BareNamespace(),
		Versions:  map[string]domain.ArrowVersion{key: av},
	}
}

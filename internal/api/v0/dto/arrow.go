package dto

import (
	"github.com/rabbytesoftware/quiver.core/internal/app/hub"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type ArrowDTO struct {
	Namespace     string            `json:"namespace"`
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	Tags          []string          `json:"tags"`
	Media         domain.ArrowMedia `json:"media"`
	UserInstalled bool              `json:"user_installed"`
	LastUsedAt    string            `json:"last_used_at,omitempty"`
}

func ArrowDTOFrom(a domain.Arrow) ArrowDTO {
	lastUsedAt := ""
	if !a.LastUsedAt.IsZero() {
		lastUsedAt = a.LastUsedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	return ArrowDTO{
		Namespace:     string(a.Namespace),
		Name:          a.Name,
		Description:   a.Description,
		Tags:          a.Tags,
		Media:         a.Media,
		UserInstalled: a.UserInstalled,
		LastUsedAt:    lastUsedAt,
	}
}

type arrowEventDTO struct {
	Event string `json:"event"`
	ArrowDTO
}

func ArrowEventDTOFrom(evt hub.ArrowEvent) arrowEventDTO {
	if evt.Kind == hub.CatalogRemoved {
		return arrowEventDTO{
			Event:    "removed",
			ArrowDTO: ArrowDTO{Namespace: string(evt.Namespace)},
		}
	}
	return arrowEventDTO{
		Event:    "upserted",
		ArrowDTO: ArrowDTOFrom(evt.Arrow),
	}
}

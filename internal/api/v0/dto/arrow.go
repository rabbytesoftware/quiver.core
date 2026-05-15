package dto

import (
	"github.com/rabbytesoftware/quiver.core/internal/app/hub"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type ArrowDTO struct {
	Namespace     string           `json:"namespace"`
	Name          string           `json:"name"`
	Version       string           `json:"version"`
	Description   string           `json:"description"`
	Tags          []string         `json:"tags"`
	Media         domain.ArrowMedia `json:"media"`
	UserInstalled bool             `json:"user_installed"`
}

func ArrowDTOFrom(a domain.Arrow) ArrowDTO {
	return ArrowDTO{
		Namespace:     string(a.Namespace),
		Name:          a.Name,
		Version:       a.Version,
		Description:   a.Description,
		Tags:          a.Tags,
		Media:         a.Media,
		UserInstalled: a.UserInstalled,
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

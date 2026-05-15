package dto

import (
	"github.com/rabbytesoftware/quiver.core/internal/app/hub"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type ArrowDTO struct {
	Namespace     string   `json:"namespace"`
	Name          string   `json:"name"`
	Version       string   `json:"version"`
	Description   string   `json:"description"`
	Tags          []string `json:"tags"`
	UserInstalled bool     `json:"user_installed"`
}

func ArrowDTOFrom(a domain.Arrow) ArrowDTO {
	return ArrowDTO{
		Namespace:     string(a.Namespace),
		Name:          a.Name,
		Version:       a.Version,
		Description:   a.Description,
		Tags:          a.Tags,
		UserInstalled: a.UserInstalled,
	}
}

type arrowUpsertedEventDTO struct {
	Event string `json:"event"`
	ArrowDTO
}

type removedEventDTO struct {
	Event     string `json:"event"`
	Namespace string `json:"namespace"`
}

func ArrowEventDTOFrom(evt hub.ArrowEvent) any {
	if evt.Kind == hub.CatalogRemoved {
		return removedEventDTO{
			Event:     "removed",
			Namespace: string(evt.Namespace),
		}
	}
	return arrowUpsertedEventDTO{
		Event:    "upserted",
		ArrowDTO: ArrowDTOFrom(evt.Arrow),
	}
}

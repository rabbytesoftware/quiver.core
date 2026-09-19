package dto

import (
	"github.com/rabbytesoftware/quiver.core/internal/app/hub"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type QuiverDTO struct {
	Namespace   string   `json:"namespace" yaml:"namespace"`
	Name        string   `json:"name" yaml:"name"`
	Description string   `json:"description" yaml:"description"`
	Tags        []string `json:"tags" yaml:"tags"`
	Followed    bool     `json:"followed" yaml:"followed"`
}

func QuiverDTOFrom(q domain.Collection) QuiverDTO {
	return QuiverDTO{
		Namespace:   string(q.Namespace),
		Name:        q.Meta.Name,
		Description: q.Meta.Description,
		Tags:        q.Meta.Tags,
		Followed:    !q.FollowedAt.IsZero(),
	}
}

type collectionEventDTO struct {
	Event string `json:"event" yaml:"event"`
	QuiverDTO
}

func CollectionEventDTOFrom(evt hub.CollectionEvent) collectionEventDTO {
	if evt.Kind == hub.CatalogRemoved {
		return collectionEventDTO{
			Event:     "removed",
			QuiverDTO: QuiverDTO{Namespace: string(evt.Namespace)},
		}
	}
	return collectionEventDTO{
		Event:     "upserted",
		QuiverDTO: QuiverDTOFrom(evt.Collection),
	}
}

package dto

import (
	"github.com/rabbytesoftware/quiver.core/internal/app/hub"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type QuiverDTO struct {
	Namespace   string   `json:"namespace"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Followed    bool     `json:"followed"`
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

type collectionUpsertedEventDTO struct {
	Event string `json:"event"`
	QuiverDTO
}

func CollectionEventDTOFrom(evt hub.CollectionEvent) any {
	if evt.Kind == hub.CatalogRemoved {
		return removedEventDTO{
			Event:     "removed",
			Namespace: string(evt.Namespace),
		}
	}
	return collectionUpsertedEventDTO{
		Event:     "upserted",
		QuiverDTO: QuiverDTOFrom(evt.Collection),
	}
}

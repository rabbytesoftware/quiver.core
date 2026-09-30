package dto

import (
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/app/models"
)

// HomeShelfDTO is one named list of arrows on the home screen. Title is the
// only label a client should show: a shelf is a group of recommendations, not a
// listing of any one host. RefreshedAt is null for a shelf that has never been
// filled.
type HomeShelfDTO struct {
	ID          string            `json:"id" yaml:"id"`
	Title       string            `json:"title" yaml:"title"`
	RefreshedAt *time.Time        `json:"refreshed_at" yaml:"refreshed_at"`
	Arrows      []SearchResultDTO `json:"arrows" yaml:"arrows"`
}

// HomeDTO is what GET /v0/home returns: every shelf from the local snapshot,
// and whether a refresh is running that may still change them.
type HomeDTO struct {
	Shelves    []HomeShelfDTO `json:"shelves" yaml:"shelves"`
	Refreshing bool           `json:"refreshing" yaml:"refreshing"`
}

func HomeDTOFrom(
	home models.Home,
) HomeDTO {
	shelves := make([]HomeShelfDTO, 0, len(home.Shelves))
	for _, shelf := range home.Shelves {
		shelves = append(shelves, homeShelfDTOFrom(shelf))
	}
	return HomeDTO{Shelves: shelves, Refreshing: home.Refreshing}
}

func homeShelfDTOFrom(
	shelf models.HomeShelf,
) HomeShelfDTO {
	arrows := make([]SearchResultDTO, 0, len(shelf.Arrows))
	for _, arrow := range shelf.Arrows {
		arrows = append(arrows, SearchResultDTOFrom(arrow))
	}

	var refreshedAt *time.Time
	if !shelf.RefreshedAt.IsZero() {
		at := shelf.RefreshedAt.UTC()
		refreshedAt = &at
	}
	return HomeShelfDTO{ID: shelf.ID, Title: shelf.Title, RefreshedAt: refreshedAt, Arrows: arrows}
}

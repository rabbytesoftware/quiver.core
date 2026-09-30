package usecases

import (
	"context"
	"fmt"

	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/recommendation"
)

// HomeUsecase serves the query-less shelves of the home screen. It answers from
// the local snapshot and never touches the network.
type HomeUsecase interface {
	Home(
		ctx context.Context,
	) (models.Home, error)

	// Refresh asks for the shelves to be rebuilt in the background and returns
	// at once; a refresh already running is joined, not restarted.
	Refresh(
		ctx context.Context,
	)
}

type homeUsecase struct {
	recommendations recommendation.Recommendation
}

func NewHomeUsecase(
	recommendations recommendation.Recommendation,
) HomeUsecase {
	return &homeUsecase{recommendations: recommendations}
}

func (h *homeUsecase) Home(
	ctx context.Context,
) (models.Home, error) {
	shelves, err := h.recommendations.Home(ctx)
	if err != nil {
		return models.Home{}, fmt.Errorf("home: %w", err)
	}

	out := make([]models.HomeShelf, 0, len(shelves))
	for _, shelf := range shelves {
		out = append(out, homeShelf(shelf))
	}
	return models.Home{Shelves: out, Refreshing: h.recommendations.Refreshing()}, nil
}

func (h *homeUsecase) Refresh(
	ctx context.Context,
) {
	h.recommendations.Refresh(ctx)
}

func homeShelf(
	shelf recommendation.Shelf,
) models.HomeShelf {
	arrows := make([]models.SearchResult, 0, len(shelf.Entries))
	for _, entry := range shelf.Entries {
		arrows = append(arrows, homeArrow(entry))
	}
	return models.HomeShelf{ID: shelf.ID, Title: shelf.Title, RefreshedAt: shelf.RefreshedAt, Arrows: arrows}
}

// homeArrow renders an entry the way a vault-sourced search result renders, so
// the home and GET /v0/search describe the same arrow the same way. An arrow the
// catalog holds is installed and carries no provenance, as in a streamed
// discovery result: the snapshot cannot say how the catalog came to hold it.
func homeArrow(
	entry recommendation.Entry,
) models.SearchResult {
	result := seenResult(entry.Namespace, entry.Rows[0], nil)
	result.Stars = entry.Stars
	result.Source = entry.Source
	result.Versions = sortedRefs(rowRefs(entry))

	if entry.InCatalog {
		result.Installed = true
		result.Provenance = ""
	}
	return result
}

func rowRefs(
	entry recommendation.Entry,
) []string {
	refs := make([]string, 0, len(entry.Rows))
	for _, row := range entry.Rows {
		refs = append(refs, row.Ref)
	}
	return refs
}

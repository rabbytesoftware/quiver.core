package dto_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestHomeDTOFrom_MapsShelvesArrowsAndTheFlag(t *testing.T) {
	at := time.Date(2026, 9, 30, 12, 0, 0, 0, time.FixedZone("ART", -3*3600))

	d := dto.HomeDTOFrom(models.Home{
		Refreshing: true,
		Shelves: []models.HomeShelf{{
			ID:          "popular",
			Title:       "Popular",
			RefreshedAt: at,
			Arrows:      []models.SearchResult{{Namespace: domain.Namespace("github.com/a/one"), Name: "One", Stars: 7}},
		}},
	})

	assert.True(t, d.Refreshing)
	require.Len(t, d.Shelves, 1)
	assert.Equal(t, "popular", d.Shelves[0].ID)
	assert.Equal(t, "Popular", d.Shelves[0].Title)
	require.NotNil(t, d.Shelves[0].RefreshedAt)
	assert.True(t, at.Equal(*d.Shelves[0].RefreshedAt))
	assert.Equal(t, time.UTC, d.Shelves[0].RefreshedAt.Location())
	require.Len(t, d.Shelves[0].Arrows, 1)
	assert.Equal(t, "github.com/a/one", d.Shelves[0].Arrows[0].Namespace)
	assert.Equal(t, 7, d.Shelves[0].Arrows[0].Stars)
}

func TestHomeDTOFrom_PinnedWireShape(t *testing.T) {
	d := dto.HomeDTOFrom(models.Home{
		Shelves: []models.HomeShelf{
			{ID: "popular", Title: "Popular", RefreshedAt: time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)},
			{ID: "fresh", Title: "Recently updated"},
		},
	})

	raw, err := json.Marshal(d)
	require.NoError(t, err)

	assert.JSONEq(t, `{
		"shelves": [
			{"id": "popular", "title": "Popular", "refreshed_at": "2026-09-30T12:00:00Z", "arrows": []},
			{"id": "fresh", "title": "Recently updated", "refreshed_at": null, "arrows": []}
		],
		"refreshing": false
	}`, string(raw))
}

func TestHomeDTOFrom_NoShelves_RendersAnEmptyListNotNull(t *testing.T) {
	raw, err := json.Marshal(dto.HomeDTOFrom(models.Home{}))
	require.NoError(t, err)

	assert.JSONEq(t, `{"shelves": [], "refreshing": false}`, string(raw))
}

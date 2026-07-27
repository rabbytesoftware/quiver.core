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

func TestArrowListItemDTOFrom(t *testing.T) {
	installedAt := time.Date(2026, 4, 11, 15, 33, 0, 0, time.UTC)
	a := models.ArrowListDTO{
		Namespace:   domain.Namespace("github.com/user/repo"),
		Name:        "My Arrow",
		Description: "desc",
		Tags:        []string{"tag1"},
		Versions: []models.InstalledVersionDTO{
			{
				Ref:         "v1.0.0",
				State:       domain.ArrowStateReady,
				InstalledAt: installedAt,
				Constraint:  "^1.0.0",
			},
		},
	}
	d := dto.ArrowListItemDTOFrom(a)
	assert.Equal(t, "github.com/user/repo", d.Namespace)
	assert.Equal(t, "My Arrow", d.Name)
	assert.Equal(t, "desc", d.Description)
	assert.Equal(t, []string{"tag1"}, d.Tags)
	require.Len(t, d.Versions, 1)
	assert.Equal(t, "v1.0.0", d.Versions[0].Ref)
	assert.Equal(t, "ready", d.Versions[0].State)
	assert.Equal(t, "2026-04-11T15:33:00Z", d.Versions[0].InstalledAt)
	assert.Equal(t, "^1.0.0", d.Versions[0].Constraint)
}

func TestArrowListItemDTOFrom_MediaMapped(t *testing.T) {
	a := models.ArrowListDTO{
		Namespace: domain.Namespace("github.com/user/repo"),
		Name:      "My Arrow",
		Media:     domain.ArrowMedia{Icon: "https://example.com/icon.png", Banner: "https://example.com/banner.png"},
	}
	d := dto.ArrowListItemDTOFrom(a)
	assert.Equal(t, "https://example.com/icon.png", d.Media.Icon)
	assert.Equal(t, "https://example.com/banner.png", d.Media.Banner)
}

// A version row's ref reaches the client under one name. `version` was a second
// wire name for the same fact and is gone; the row's own ref answers both, so a
// client reading either key must find `ref` and nothing beside it.
func TestInstalledVersionItemDTO_WireShape(t *testing.T) {
	d := dto.ArrowListItemDTOFrom(models.ArrowListDTO{
		Namespace: domain.Namespace("github.com/user/repo"),
		Versions: []models.InstalledVersionDTO{
			{
				Ref:         "v1.0.0",
				State:       domain.ArrowStateReady,
				InstalledAt: time.Date(2026, 4, 11, 15, 33, 0, 0, time.UTC),
			},
		},
	})
	require.Len(t, d.Versions, 1)

	blob, err := json.Marshal(d.Versions[0])
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(blob, &decoded))

	assert.NotContains(t, decoded, "version")
	assert.Equal(t, "v1.0.0", decoded["ref"])
	assert.Equal(t, "ready", decoded["state"])
	assert.Equal(t, "2026-04-11T15:33:00Z", decoded["installed_at"])
}

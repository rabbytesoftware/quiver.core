package dto_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/app/hub"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestArrowEventDTOFrom_Upserted(t *testing.T) {
	evt := hub.ArrowEvent{
		Kind: hub.CatalogUpserted,
		Arrow: domain.Arrow{
			Namespace:     "github.com/user/repo@v1.0.0",
			ArrowMeta:     domain.ArrowMeta{Name: "repo", Version: "v1.0.0", Description: "desc", Tags: []string{"a"}},
			UserInstalled: true,
		},
	}
	data, err := json.Marshal(dto.ArrowEventDTOFrom(evt))
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))

	assert.Equal(t, "upserted", m["event"])
	assert.Equal(t, "github.com/user/repo@v1.0.0", m["namespace"])
	assert.Equal(t, "repo", m["name"])
	assert.Equal(t, "v1.0.0", m["version"])
	assert.Equal(t, "desc", m["description"])
	assert.Equal(t, true, m["user_installed"])
}

func TestArrowEventDTOFrom_Removed(t *testing.T) {
	evt := hub.ArrowEvent{
		Kind:  hub.CatalogRemoved,
		Arrow: domain.Arrow{Namespace: "github.com/user/repo@v1.0.0"},
	}
	data, err := json.Marshal(dto.ArrowEventDTOFrom(evt))
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))

	assert.Equal(t, "removed", m["event"])
	assert.Equal(t, "github.com/user/repo@v1.0.0", m["namespace"])
}

func TestArrowDTOFrom(t *testing.T) {
	a := domain.Arrow{
		Namespace: "github.com/user/repo",
		ArrowMeta: domain.ArrowMeta{Name: "Test", Version: "1.0.0"},
	}
	d := dto.ArrowDTOFrom(a)
	assert.Equal(t, "github.com/user/repo", d.Namespace)
	assert.Equal(t, "Test", d.Name)
	assert.Equal(t, "1.0.0", d.Version)

	data, err := json.Marshal(d)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"namespace"`)

	// user_installed must be propagated so arrowWatcher can filter correctly.
	aInstalled := domain.Arrow{
		Namespace:     "github.com/user/repo",
		ArrowMeta:     domain.ArrowMeta{Name: "Test", Version: "1.0.0"},
		UserInstalled: true,
	}
	dInstalled := dto.ArrowDTOFrom(aInstalled)
	assert.True(t, dInstalled.UserInstalled, "UserInstalled must be propagated to ArrowDTO")

	dataInstalled, err := json.Marshal(dInstalled)
	require.NoError(t, err)
	assert.Contains(t, string(dataInstalled), `"user_installed":true`)
}

func TestArrowDTOFrom_MediaMapped(t *testing.T) {
	a := domain.Arrow{
		Namespace: "github.com/user/repo",
		ArrowMeta: domain.ArrowMeta{
			Name:    "Test",
			Version: "1.0.0",
			Media:   domain.ArrowMedia{Icon: "https://example.com/icon.png", Banner: "https://example.com/banner.png"},
		},
	}
	d := dto.ArrowDTOFrom(a)
	assert.Equal(t, "https://example.com/icon.png", d.Media.Icon)
	assert.Equal(t, "https://example.com/banner.png", d.Media.Banner)
}

func TestArrowEventDTOFrom_UpsertedIncludesMedia(t *testing.T) {
	evt := hub.ArrowEvent{
		Kind: hub.CatalogUpserted,
		Arrow: domain.Arrow{
			Namespace: "github.com/user/repo@v1.0.0",
			ArrowMeta: domain.ArrowMeta{
				Name:    "repo",
				Version: "v1.0.0",
				Media:   domain.ArrowMedia{Icon: "https://example.com/icon.png", Banner: "https://example.com/banner.png"},
			},
		},
	}
	data, err := json.Marshal(dto.ArrowEventDTOFrom(evt))
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))

	media, ok := m["media"].(map[string]any)
	require.True(t, ok, "media key must be present")
	assert.Equal(t, "https://example.com/icon.png", media["icon"])
	assert.Equal(t, "https://example.com/banner.png", media["banner"])
}

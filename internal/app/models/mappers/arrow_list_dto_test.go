package mappers_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/app/models/mappers"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestArrowListDTOsFrom_Empty(t *testing.T) {
	result := mappers.ArrowListDTOsFrom(nil)
	assert.Empty(t, result)
}

func TestArrowListDTOsFrom_MediaMapped(t *testing.T) {
	views := []models.ArrowView{
		{
			Namespace: "github.com/org/repo",
			Metadata: domain.Arrow{
				ArrowMeta: domain.ArrowMeta{
					Name:  "Repo",
					Media: domain.ArrowMedia{Icon: "https://example.com/icon.png", Banner: "https://example.com/banner.png"},
				},
			},
		},
	}
	result := mappers.ArrowListDTOsFrom(views)
	assert.Len(t, result, 1)
	assert.Equal(t, "https://example.com/icon.png", result[0].Media.Icon)
	assert.Equal(t, "https://example.com/banner.png", result[0].Media.Banner)
}

func TestArrowListDTOsFrom_MapsVersions(t *testing.T) {
	at := time.Now().UTC()
	views := []models.ArrowView{
		{
			Namespace: "github.com/org/repo",
			Metadata: domain.Arrow{
				ArrowMeta: domain.ArrowMeta{Name: "Repo", Description: "desc", Tags: []string{"tag1"}},
			},
			Versions: []models.VersionView{
				{
					Namespace: "github.com/org/repo@v1.0.0",
					State:     domain.ArrowStateReady,
					Metadata: domain.Arrow{
						ArrowMeta:           domain.ArrowMeta{Version: "1.0.0"},
						InstalledRef:        "v1.0.0",
						InstalledAt:         at,
						InstalledConstraint: "^1.0.0",
					},
				},
			},
		},
	}

	result := mappers.ArrowListDTOsFrom(views)

	assert.Len(t, result, 1)
	dto := result[0]
	assert.Equal(t, domain.Namespace("github.com/org/repo"), dto.Namespace)
	assert.Equal(t, "Repo", dto.Name)
	assert.Equal(t, "desc", dto.Description)
	assert.Equal(t, []string{"tag1"}, dto.Tags)
	assert.Len(t, dto.Versions, 1)
	ver := dto.Versions[0]
	assert.Equal(t, "v1.0.0", ver.Ref)
	assert.Equal(t, "1.0.0", ver.Version)
	assert.Equal(t, domain.ArrowStateReady, ver.State)
	assert.Equal(t, at, ver.InstalledAt)
	assert.Equal(t, "^1.0.0", ver.Constraint)
}

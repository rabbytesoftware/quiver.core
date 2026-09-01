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
	lastUsed := at.Add(time.Hour)
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
						InstalledAt:         at,
						LastUsedAt:          lastUsed,
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
	assert.Equal(t, domain.ArrowStateReady, ver.State)
	assert.Equal(t, at, ver.InstalledAt)
	assert.Equal(t, lastUsed, ver.LastUsedAt)
	assert.Equal(t, "^1.0.0", ver.Constraint)
}

// A catalog row that has never been installed still names the ref it is filed
// under; that it is not on disk is State and InstalledAt's to report, and the
// Ref must not go empty to say it.
func TestArrowListDTOsFrom_UninstalledVersionStillNamesItsRef(t *testing.T) {
	views := []models.ArrowView{
		{
			Namespace: "github.com/org/repo",
			Versions: []models.VersionView{
				{
					Namespace: "github.com/org/repo@v2.0.0",
					State:     domain.ArrowStateAbsent,
					Metadata:  domain.Arrow{},
				},
			},
		},
	}

	result := mappers.ArrowListDTOsFrom(views)

	assert.Len(t, result, 1)
	assert.Len(t, result[0].Versions, 1)
	ver := result[0].Versions[0]
	assert.Equal(t, "v2.0.0", ver.Ref)
	assert.Equal(t, domain.ArrowStateAbsent, ver.State)
	assert.True(t, ver.InstalledAt.IsZero())
	assert.True(t, ver.LastUsedAt.IsZero())
}

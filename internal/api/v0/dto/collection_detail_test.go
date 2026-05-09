package dto_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestCollectionDetailDTOFrom(t *testing.T) {
	q := &models.CollectionDetailDTO{
		Namespace:   domain.Namespace("github.com/user/repo"),
		Name:        "My Quiver",
		Version:     "v1.2.3",
		Description: "A description",
		URL:         "https://example.com",
		Maintainers: []string{"alice", "bob"},
		Tags:        []string{"tag1", "tag2"},
		Media:       domain.CollectionMedia{Icon: "icon.png", Banner: "banner.png"},
		Arrows: []models.CollectionArrowDTO{
			{
				Namespace:   domain.Namespace("github.com/user/arrow"),
				Resolved:    true,
				Name:        "My Arrow",
				Version:     "v0.1.0",
				Description: "Arrow desc",
			},
		},
		Followed: true,
	}

	d := dto.CollectionDetailDTOFrom(q)

	assert.Equal(t, "github.com/user/repo", d.Namespace)
	assert.Equal(t, "My Quiver", d.Name)
	assert.Equal(t, "v1.2.3", d.Version)
	assert.Equal(t, "A description", d.Description)
	assert.Equal(t, "https://example.com", d.URL)
	assert.Equal(t, []string{"alice", "bob"}, d.Maintainers)
	assert.Equal(t, []string{"tag1", "tag2"}, d.Tags)
	assert.Equal(t, "icon.png", d.Media.Icon)
	assert.Equal(t, "banner.png", d.Media.Banner)
	assert.True(t, d.Followed)
	assert.Len(t, d.Arrows, 1)
	assert.Equal(t, "github.com/user/arrow", d.Arrows[0].Namespace)
	assert.True(t, d.Arrows[0].Resolved)
	assert.Equal(t, "My Arrow", d.Arrows[0].Name)
	assert.Equal(t, "v0.1.0", d.Arrows[0].Version)
	assert.Equal(t, "Arrow desc", d.Arrows[0].Description)
}

func TestCollectionDetailDTOFrom_EmptyArrows(t *testing.T) {
	q := &models.CollectionDetailDTO{
		Namespace: domain.Namespace("github.com/user/repo"),
		Name:      "My Quiver",
	}

	d := dto.CollectionDetailDTOFrom(q)

	assert.Equal(t, "github.com/user/repo", d.Namespace)
	assert.NotNil(t, d.Arrows)
	assert.Len(t, d.Arrows, 0)
	assert.False(t, d.Followed)
}

func TestCollectionListItemDTOFrom(t *testing.T) {
	q := models.CollectionListDTO{
		Namespace:   domain.Namespace("github.com/user/repo"),
		Name:        "My Quiver",
		Description: "A description",
		Tags:        []string{"tag1"},
		ArrowCount:  5,
		Followed:    true,
	}

	d := dto.CollectionListItemDTOFrom(q)

	assert.Equal(t, "github.com/user/repo", d.Namespace)
	assert.Equal(t, "My Quiver", d.Name)
	assert.Equal(t, "A description", d.Description)
	assert.Equal(t, []string{"tag1"}, d.Tags)
	assert.Equal(t, 5, d.ArrowCount)
	assert.True(t, d.Followed)
}

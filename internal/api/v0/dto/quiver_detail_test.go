package dto_test

import (
	"testing"

	"github.com/rabbytesoftware/quiver/internal/api/v0/dto"
	appquiver "github.com/rabbytesoftware/quiver/internal/app/quiver"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestQuiverDetailDTOFrom(t *testing.T) {
	q := &appquiver.QuiverDetailDTO{
		Namespace: domain.Namespace("github.com/user/repo"),
		Manifest: domain.QuiverManifest{
			Name:        "My Quiver",
			Description: "A description",
			URL:         "https://example.com",
			Tags:        []string{"tag1", "tag2"},
		},
	}

	d := dto.QuiverDetailDTOFrom(q)

	assert.Equal(t, "github.com/user/repo", d.Namespace)
	assert.Equal(t, "My Quiver", d.Name)
	assert.Equal(t, "A description", d.Description)
	assert.Equal(t, "https://example.com", d.URL)
	assert.Equal(t, []string{"tag1", "tag2"}, d.Tags)
}

func TestQuiverListItemDTOFrom(t *testing.T) {
	q := appquiver.QuiverListDTO{
		Namespace:   domain.Namespace("github.com/user/repo"),
		Name:        "My Quiver",
		Description: "A description",
		Tags:        []string{"tag1"},
	}

	d := dto.QuiverListItemDTOFrom(q)

	assert.Equal(t, "github.com/user/repo", d.Namespace)
	assert.Equal(t, "My Quiver", d.Name)
	assert.Equal(t, "A description", d.Description)
	assert.Equal(t, []string{"tag1"}, d.Tags)
}

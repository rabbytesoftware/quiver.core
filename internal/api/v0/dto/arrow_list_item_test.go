package dto_test

import (
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver/internal/app/arrow"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArrowListItemDTOFrom(t *testing.T) {
	installedAt := time.Date(2026, 4, 11, 15, 33, 0, 0, time.UTC)
	a := arrow.ArrowListDTO{
		Namespace:   domain.Namespace("github.com/user/repo"),
		Name:        "My Arrow",
		Description: "desc",
		Tags:        []string{"tag1"},
		Versions: []arrow.InstalledVersionDTO{
			{
				Ref:         "v1.0.0",
				Version:     "1.0.0",
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
	assert.Equal(t, "1.0.0", d.Versions[0].Version)
	assert.Equal(t, "ready", d.Versions[0].State)
	assert.Equal(t, "^1.0.0", d.Versions[0].Constraint)
}

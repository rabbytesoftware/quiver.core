package dto_test

import (
	"testing"

	"github.com/rabbytesoftware/quiver/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver/internal/app/arrow"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestArrowListItemDTOFrom(t *testing.T) {
	a := arrow.ArrowListDTO{
		Namespace:   domain.Namespace("github.com/user/repo"),
		Name:        "My Arrow",
		Version:     "1.0.0",
		Description: "desc",
		State:       domain.ArrowStateReady,
		Tags:        []string{"tag1"},
	}
	d := dto.ArrowListItemDTOFrom(a)
	assert.Equal(t, "github.com/user/repo", d.Namespace)
	assert.Equal(t, "My Arrow", d.Name)
	assert.Equal(t, "1.0.0", d.Version)
	assert.Equal(t, "ready", d.State)
	assert.Equal(t, []string{"tag1"}, d.Tags)
}

package dto_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestCollectionDetailDTOFrom(t *testing.T) {
	q := &models.CollectionDetailDTO{
		Namespace:   domain.Namespace("github.com/user/repo@v1.2.3"),
		Name:        "My Quiver",
		Description: "A description",
		URL:         "https://example.com",
		Maintainers: []string{"alice", "bob"},
		Tags:        []string{"tag1", "tag2"},
		Media:       domain.CollectionMedia{Icon: "icon.png", Banner: "banner.png"},
		Arrows: []models.CollectionArrowDTO{
			{
				Namespace:   domain.Namespace("github.com/user/arrow@v0.1.0"),
				Resolved:    true,
				Name:        "My Arrow",
				Description: "Arrow desc",
			},
		},
		Followed: true,
	}

	d := dto.CollectionDetailDTOFrom(q)

	assert.Equal(t, "github.com/user/repo@v1.2.3", d.Namespace)
	assert.Equal(t, "My Quiver", d.Name)
	assert.Equal(t, "A description", d.Description)
	assert.Equal(t, "https://example.com", d.URL)
	assert.Equal(t, []string{"alice", "bob"}, d.Maintainers)
	assert.Equal(t, []string{"tag1", "tag2"}, d.Tags)
	assert.Equal(t, "icon.png", d.Media.Icon)
	assert.Equal(t, "banner.png", d.Media.Banner)
	assert.True(t, d.Followed)
	assert.Len(t, d.Arrows, 1)
	assert.Equal(t, "github.com/user/arrow@v0.1.0", d.Arrows[0].Namespace)
	assert.True(t, d.Arrows[0].Resolved)
	assert.Equal(t, "My Arrow", d.Arrows[0].Name)
	assert.Equal(t, "Arrow desc", d.Arrows[0].Description)
}

// A collection is a curated list; the list names no artifact, so the detail
// response carries no `version` for it. A member's revision is the `@ref` on the
// namespace it already ships — a second key restating that ref would be absent
// on unresolved members and present on resolved ones, saying two things about
// one fact. Both must stay gone at both layers.
//
// The field check is what bites: a reintroduced `version` would carry
// `omitempty`, so it would sit on the type unnoticed by any assertion that only
// reads a marshalled value it has no way to populate.
func TestCollectionDTOs_DeclareNoVersionField(t *testing.T) {
	types := map[string]reflect.Type{
		"dto.CollectionDetailDTO":    reflect.TypeOf(dto.CollectionDetailDTO{}),
		"dto.CollectionArrowDTO":     reflect.TypeOf(dto.CollectionArrowDTO{}),
		"models.CollectionDetailDTO": reflect.TypeOf(models.CollectionDetailDTO{}),
		"models.CollectionArrowDTO":  reflect.TypeOf(models.CollectionArrowDTO{}),
		"models.CollectionListDTO":   reflect.TypeOf(models.CollectionListDTO{}),
	}
	for name, typ := range types {
		_, found := typ.FieldByName("Version")
		assert.False(t, found, "%s must not declare a Version field", name)
	}
}

func TestCollectionDetailDTO_WireShape_NoVersionKey(t *testing.T) {
	blob, err := json.Marshal(dto.CollectionDetailDTOFrom(&models.CollectionDetailDTO{
		Namespace:   domain.Namespace("github.com/user/repo@v1.2.3"),
		Name:        "My Quiver",
		Description: "A description",
		Arrows: []models.CollectionArrowDTO{
			{
				Namespace:   domain.Namespace("github.com/user/arrow@v0.1.0"),
				Resolved:    true,
				Name:        "My Arrow",
				Description: "Arrow desc",
			},
			{Namespace: domain.Namespace("github.com/user/broken@v2")},
		},
	}))
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(blob, &decoded))

	assert.NotContains(t, decoded, "version")
	assert.Equal(t, "github.com/user/repo@v1.2.3", decoded["namespace"])

	arrows, ok := decoded["arrows"].([]any)
	require.True(t, ok, "detail = %s, want an arrows array", blob)
	require.Len(t, arrows, 2)

	resolved, ok := arrows[0].(map[string]any)
	require.True(t, ok)
	assert.NotContains(t, resolved, "version")
	assert.Equal(t, "github.com/user/arrow@v0.1.0", resolved["namespace"])
	assert.Equal(t, true, resolved["resolved"])

	unresolved, ok := arrows[1].(map[string]any)
	require.True(t, ok)
	assert.NotContains(t, unresolved, "version")
	assert.Equal(t, "github.com/user/broken@v2", unresolved["namespace"])
	assert.Equal(t, false, unresolved["resolved"])
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

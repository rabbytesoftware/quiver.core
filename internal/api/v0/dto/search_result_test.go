package dto_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestSearchResultDTOFrom(t *testing.T) {
	r := models.SearchResult{
		Namespace:    domain.Namespace("github.com/user/repo"),
		Name:         "My Arrow",
		Description:  "desc",
		Tags:         []string{"db", "cache"},
		Icon:         "icon.png",
		Banner:       "banner.png",
		Versions:     []string{"v1.0.0", "v2.0.0"},
		CompatibleOS: []domain.OS{domain.OSLinuxAMD64, domain.OSDarwinARM64},
		Provenance:   models.ProvenanceCollection,
		Installed:    true,
		Stars:        7,
		Source:       "github",
	}

	d := dto.SearchResultDTOFrom(r)

	assert.Equal(t, "github.com/user/repo", d.Namespace)
	assert.Equal(t, "My Arrow", d.Name)
	assert.Equal(t, "desc", d.Description)
	assert.Equal(t, []string{"db", "cache"}, d.Tags)
	assert.Equal(t, domain.ArrowMedia{Icon: "icon.png", Banner: "banner.png"}, d.Media)
	assert.Equal(t, []string{"v1.0.0", "v2.0.0"}, d.Versions)
	assert.Equal(t, []string{"linux/amd64", "darwin/arm64"}, d.CompatibleOS)
	assert.Equal(t, "collection", d.Provenance)
	assert.True(t, d.Installed)
	assert.Equal(t, 7, d.Stars)
	assert.Equal(t, "github", d.Source)
}

func TestSearchResultDTOFrom_DiscoveredArrowKeepsListsEmptyNotNull(t *testing.T) {
	d := dto.SearchResultDTOFrom(models.SearchResult{
		Namespace:  domain.Namespace("github.com/user/unseen"),
		Provenance: models.ProvenanceSeen,
	})

	require.NotNil(t, d.Tags)
	require.NotNil(t, d.Versions)
	require.NotNil(t, d.CompatibleOS)

	encoded, err := json.Marshal(d)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"tags":[]`)
	assert.Contains(t, string(encoded), `"versions":[]`)
	assert.Contains(t, string(encoded), `"compatible_os":[]`)
	assert.NotContains(t, string(encoded), `"source"`)
}

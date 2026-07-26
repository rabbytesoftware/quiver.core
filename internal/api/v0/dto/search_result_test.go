package dto_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/discovery"
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

func TestSearchResultDTOFromDiscovery(t *testing.T) {
	d := dto.SearchResultDTOFromDiscovery(discovery.Result{
		Arrow: domain.Arrow{
			Namespace: domain.Namespace("github.com/user/repo@v1.2.3"),
			ArrowMeta: domain.ArrowMeta{
				Name:        "My Arrow",
				Description: "desc",
				Version:     "v1.2.3",
				Tags:        []string{"db", "cache"},
				Media:       domain.ArrowMedia{Icon: "icon.png", Banner: "banner.png"},
			},
			Targets: map[domain.OS]domain.Target{
				domain.OSLinuxAMD64:  {},
				domain.OSDarwinARM64: {},
			},
		},
		Namespace: domain.Namespace("github.com/user/repo"),
		Stars:     7,
		Source:    "github.com",
	})

	assert.Equal(t, "github.com/user/repo", d.Namespace)
	assert.Equal(t, "My Arrow", d.Name)
	assert.Equal(t, "desc", d.Description)
	assert.Equal(t, []string{"db", "cache"}, d.Tags)
	assert.Equal(t, domain.ArrowMedia{Icon: "icon.png", Banner: "banner.png"}, d.Media)
	assert.Equal(t, []string{"v1.2.3"}, d.Versions)
	assert.Equal(t, []string{"darwin/arm64", "linux/amd64"}, d.CompatibleOS)
	assert.Equal(t, models.ProvenanceSeen, d.Provenance)
	assert.False(t, d.Installed)
	assert.Equal(t, 7, d.Stars)
	assert.Equal(t, "github.com", d.Source)
}

// TestSearchResultDTOFromDiscovery_BareResultKeepsListsEmptyNotNull covers a
// manifest that declared no version and no targets: the client still iterates
// every list the same way.
func TestSearchResultDTOFromDiscovery_BareResultKeepsListsEmptyNotNull(t *testing.T) {
	d := dto.SearchResultDTOFromDiscovery(discovery.Result{
		Namespace: domain.Namespace("github.com/user/bare"),
	})

	require.NotNil(t, d.Tags)
	require.NotNil(t, d.Versions)
	require.NotNil(t, d.CompatibleOS)
	assert.Empty(t, d.Versions)
	assert.Empty(t, d.CompatibleOS)
	assert.Equal(t, models.ProvenanceSeen, d.Provenance)
}

// TestSearchResultDTOFromDiscovery_MatchesLaneA proves the two lanes share one
// renderer rather than two mappers that happen to agree today.
func TestSearchResultDTOFromDiscovery_MatchesLaneA(t *testing.T) {
	streamed := dto.SearchResultDTOFromDiscovery(discovery.Result{
		Arrow: domain.Arrow{
			ArrowMeta: domain.ArrowMeta{
				Name:    "My Arrow",
				Version: "v1.2.3",
				Tags:    []string{"db"},
			},
			Targets: map[domain.OS]domain.Target{domain.OSLinuxAMD64: {}},
		},
		Namespace: domain.Namespace("github.com/user/repo"),
		Stars:     7,
		Source:    "github.com",
	})

	laneA := dto.SearchResultDTOFrom(models.SearchResult{
		Namespace:    domain.Namespace("github.com/user/repo"),
		Name:         "My Arrow",
		Tags:         []string{"db"},
		Versions:     []string{"v1.2.3"},
		CompatibleOS: []domain.OS{domain.OSLinuxAMD64},
		Provenance:   models.ProvenanceSeen,
		Stars:        7,
		Source:       "github.com",
	})

	assert.Equal(t, laneA, streamed)
}

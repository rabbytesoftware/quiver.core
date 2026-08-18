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
		Media:        domain.ArrowMedia{Icon: "icon.png", Banner: "banner.png"},
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
// result whose arrow was never resolved, so it has neither a ref nor targets:
// the client still iterates every list the same way.
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
			Namespace: domain.Namespace("github.com/user/repo@v1.2.3"),
			ArrowMeta: domain.ArrowMeta{
				Name: "My Arrow",
				Tags: []string{"db"},
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

// A discovered arrow the catalog already holds must not be streamed as a
// downgrade. Claiming provenance "seen" and installed false would make a
// client that merges stream entries over its rendered list replace a correct
// installed row with a worse one, and nothing in the payload would reveal it.
func TestSearchResultDTOFromDiscovery_CatalogArrowIsNotDowngraded(t *testing.T) {
	got := dto.SearchResultDTOFromDiscovery(discovery.Result{
		Namespace: domain.Namespace("github.com/user/pkg"),
		Arrow: domain.Arrow{
			Namespace: domain.Namespace("github.com/user/pkg@v1.0.0"),
			ArrowMeta: domain.ArrowMeta{Name: "pkg"},
		},
		InCatalog: true,
	})

	assert.True(t, got.Known, "the client must be told it already has this")
	assert.True(t, got.Installed, "a catalog arrow is installed, as Lane A renders it")
	assert.Empty(t, got.Provenance,
		"discovery cannot know whether the catalog recorded installed, dependency or collection")
}

// Discovery writes every arrow it proves to the vault, so an arrow a previous
// pass indexed comes back vault-known on the next one. Installed is what the
// re-query uses to separate what the user has from what they could have, so a
// merely-cached manifest must not claim it. Lane A renders the same arrow as
// seen and not installed, and the stream has to agree.
func TestSearchResultDTOFromDiscovery_VaultOnlyArrowIsKnownButNotInstalled(t *testing.T) {
	got := dto.SearchResultDTOFromDiscovery(discovery.Result{
		Namespace: domain.Namespace("github.com/user/pkg"),
		Arrow: domain.Arrow{
			Namespace: domain.Namespace("github.com/user/pkg@v1.0.0"),
			ArrowMeta: domain.ArrowMeta{Name: "pkg"},
		},
		InVault: true,
	})

	assert.True(t, got.Known, "the client already has the manifest cached")
	assert.False(t, got.Installed, "a search cached this; nothing installed it")
	assert.Equal(t, models.ProvenanceSeen, got.Provenance)
}

func TestSearchResultDTOFromDiscovery_UnknownArrowIsSeenAndNotInstalled(t *testing.T) {
	got := dto.SearchResultDTOFromDiscovery(discovery.Result{
		Namespace: domain.Namespace("github.com/user/new"),
		Arrow: domain.Arrow{
			Namespace: domain.Namespace("github.com/user/new@v1.0.0"),
			ArrowMeta: domain.ArrowMeta{Name: "new"},
		},
	})

	assert.False(t, got.Known)
	assert.False(t, got.Installed)
	assert.Equal(t, "seen", got.Provenance)
}

// Provenance is omitempty, so a catalog result must omit the key entirely
// rather than send an empty string a client might render.
func TestSearchResultDTOFromDiscovery_CatalogArrowOmitsProvenanceKey(t *testing.T) {
	raw, err := json.Marshal(dto.SearchResultDTOFromDiscovery(discovery.Result{
		Namespace: domain.Namespace("github.com/user/pkg"),
		Arrow:     domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "pkg"}},
		InCatalog: true,
	}))

	require.NoError(t, err)
	assert.NotContains(t, string(raw), `"provenance"`)
	assert.Contains(t, string(raw), `"known":true`)
	assert.Contains(t, string(raw), `"installed":true`)
}

// Lane A renders a vault row as known, not installed, provenance seen. The
// stream renders the same arrow the same way, so a client re-querying after a
// discovery pass sees no contradiction between the two lanes.
func TestSearchResultDTOFromDiscovery_VaultOnlyMatchesLaneASeenRow(t *testing.T) {
	streamed := dto.SearchResultDTOFromDiscovery(discovery.Result{
		Namespace: domain.Namespace("github.com/user/repo"),
		Arrow: domain.Arrow{
			Namespace: domain.Namespace("github.com/user/repo@main"),
			ArrowMeta: domain.ArrowMeta{Name: "My Arrow"},
			Targets:   map[domain.OS]domain.Target{domain.OSLinuxAMD64: {}},
		},
		Stars:   7,
		Source:  "github.com",
		InVault: true,
	})

	laneA := dto.SearchResultDTOFrom(models.SearchResult{
		Namespace:    domain.Namespace("github.com/user/repo"),
		Name:         "My Arrow",
		Versions:     []string{"main"},
		CompatibleOS: []domain.OS{domain.OSLinuxAMD64},
		Provenance:   models.ProvenanceSeen,
		Known:        true,
		Stars:        7,
		Source:       "github.com",
	})

	assert.Equal(t, laneA, streamed)
}

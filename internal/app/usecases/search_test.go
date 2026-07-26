package usecases

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	ucmocks "github.com/rabbytesoftware/quiver.core/internal/app/usecases/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
	"github.com/rabbytesoftware/quiver.core/internal/mocks"
)

func catalogHit(
	ns string,
	name string,
	refs ...string,
) models.CatalogHit {
	return models.CatalogHit{
		Namespace: domain.Namespace(ns),
		Metadata: domain.Arrow{
			Namespace: domain.Namespace(ns),
			ArrowMeta: domain.ArrowMeta{
				Name:        name,
				Description: name + " description",
				Tags:        []string{"tag"},
				Media:       domain.ArrowMedia{Icon: "icon.png", Banner: "banner.png"},
			},
			Targets: map[domain.OS]domain.Target{domain.OSLinuxAMD64: {}},
		},
		Refs:       refs,
		Provenance: models.ProvenanceInstalled,
	}
}

// vaultRow mirrors the index: the namespace column is bare and the ref is its
// own column.
func vaultRow(
	ns string,
	ref string,
	name string,
	stars int,
) vault.IndexRow {
	return vault.IndexRow{
		Namespace: domain.Namespace(ns),
		Ref:       ref,
		Meta: vault.IndexMeta{
			Arrow: domain.ArrowMeta{
				Name:        name,
				Description: name + " description",
				Tags:        []string{"vault"},
				Media:       domain.ArrowMedia{Icon: "vicon.png", Banner: "vbanner.png"},
			},
			OS:     []domain.OS{domain.OSDarwinARM64},
			Stars:  stars,
			Source: "github",
		},
	}
}

func newSearch(
	hits []models.CatalogHit,
	catalogErr error,
	rows []vault.IndexRow,
	vaultErr error,
	collections []domain.Collection,
	collectionErr error,
) SearchUsecase {
	arrows := &ucmocks.MockArrow{
		SearchFn: func(_ context.Context, _ models.SearchQuery) ([]models.CatalogHit, error) {
			return hits, catalogErr
		},
	}
	v := &mocks.Vault{SearchArrowsResult: rows, SearchArrowsErr: vaultErr}
	cols := &ucmocks.MockCollection{
		ListFn: func(_ context.Context) ([]domain.Collection, error) {
			return collections, collectionErr
		},
	}
	return NewSearchUsecase(arrows, v, cols)
}

func byNamespace(
	results []models.SearchResult,
) map[domain.Namespace]models.SearchResult {
	out := make(map[domain.Namespace]models.SearchResult, len(results))
	for _, r := range results {
		out[r.Namespace] = r
	}
	return out
}

func TestSearch_EmptyQueryReturnsError(t *testing.T) {
	uc := newSearch(nil, nil, nil, nil, nil, nil)

	_, err := uc.Search(context.Background(), models.SearchQuery{Text: ""})
	require.Error(t, err)
}

func TestSearch_BlankQueryIsTreatedAsEmpty(t *testing.T) {
	uc := newSearch(nil, nil, nil, nil, nil, nil)

	_, err := uc.Search(context.Background(), models.SearchQuery{Text: "   "})
	require.Error(t, err)
}

func TestSearch_CatalogOnlyResult(t *testing.T) {
	hits := []models.CatalogHit{catalogHit("github.com/user/pkg", "pkg", "v1.0.0")}
	uc := newSearch(hits, nil, nil, nil, nil, nil)

	got, err := uc.Search(context.Background(), models.SearchQuery{Text: "pkg"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, domain.Namespace("github.com/user/pkg"), got[0].Namespace)
	assert.Equal(t, "pkg", got[0].Name)
	assert.Equal(t, "pkg description", got[0].Description)
	assert.Equal(t, []string{"tag"}, got[0].Tags)
	assert.Equal(t, "icon.png", got[0].Icon)
	assert.Equal(t, "banner.png", got[0].Banner)
	assert.Equal(t, []string{"v1.0.0"}, got[0].Versions)
	assert.Equal(t, []domain.OS{domain.OSLinuxAMD64}, got[0].CompatibleOS)
	assert.Equal(t, models.ProvenanceInstalled, got[0].Provenance)
	assert.True(t, got[0].Installed)
	assert.Zero(t, got[0].Stars)
}

func TestSearch_VaultOnlyResultIsMarkedSeenAndNotInstalled(t *testing.T) {
	rows := []vault.IndexRow{vaultRow("github.com/user/seen", "v2.0.0", "seen arrow", 12)}
	uc := newSearch(nil, nil, rows, nil, nil, nil)

	got, err := uc.Search(context.Background(), models.SearchQuery{Text: "seen"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, domain.Namespace("github.com/user/seen"), got[0].Namespace)
	assert.Equal(t, "seen arrow", got[0].Name)
	assert.Equal(t, "vicon.png", got[0].Icon)
	assert.Equal(t, "vbanner.png", got[0].Banner)
	assert.Equal(t, models.ProvenanceSeen, got[0].Provenance)
	assert.False(t, got[0].Installed)
	assert.Equal(t, 12, got[0].Stars)
	assert.Equal(t, "github", got[0].Source)
	assert.Equal(t, []domain.OS{domain.OSDarwinARM64}, got[0].CompatibleOS)
	assert.Equal(t, []string{"v2.0.0"}, got[0].Versions)
}

func TestSearch_CollisionReturnsCatalogRowOnce(t *testing.T) {
	hits := []models.CatalogHit{catalogHit("github.com/user/pkg", "pkg", "v1.0.0")}
	rows := []vault.IndexRow{vaultRow("github.com/user/pkg", "v9.9.9", "vault pkg", 9999)}
	uc := newSearch(hits, nil, rows, nil, nil, nil)

	got, err := uc.Search(context.Background(), models.SearchQuery{Text: "pkg"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "pkg", got[0].Name)
	assert.True(t, got[0].Installed)
	assert.Equal(t, []string{"v1.0.0"}, got[0].Versions, "the vault row contributes nothing")
	assert.Zero(t, got[0].Stars)
}

func TestSearch_VaultRefsCollapseToOneResultWithAllVersions(t *testing.T) {
	rows := []vault.IndexRow{
		vaultRow("github.com/user/many", "v2.0.0", "many", 3),
		vaultRow("github.com/user/many", "v1.0.0", "many", 3),
		vaultRow("github.com/user/many", "v3.0.0", "many", 3),
	}
	uc := newSearch(nil, nil, rows, nil, nil, nil)

	got, err := uc.Search(context.Background(), models.SearchQuery{Text: "many"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, domain.Namespace("github.com/user/many"), got[0].Namespace)
	assert.Equal(t, []string{"v1.0.0", "v2.0.0", "v3.0.0"}, got[0].Versions)
}

func TestSearch_FollowedCollectionMemberGetsCollectionProvenance(t *testing.T) {
	hits := []models.CatalogHit{
		catalogHit("github.com/user/member", "member", "v1.0.0"),
		catalogHit("github.com/user/stranger", "stranger", "v1.0.0"),
	}
	collections := []domain.Collection{{
		Namespace: domain.Namespace("github.com/org/list"),
		Arrows: []domain.CollectionArrow{
			{Namespace: domain.Namespace("github.com/user/member@v1.0.0")},
		},
	}}
	uc := newSearch(hits, nil, nil, nil, collections, nil)

	got, err := uc.Search(context.Background(), models.SearchQuery{Text: "thing"})
	require.NoError(t, err)
	require.Len(t, got, 2)
	indexed := byNamespace(got)
	assert.Equal(t, models.ProvenanceCollection, indexed["github.com/user/member"].Provenance)
	assert.Equal(t, models.ProvenanceInstalled, indexed["github.com/user/stranger"].Provenance)
}

func TestSearch_FollowedCollectionMemberOutranksLaterStrangers(t *testing.T) {
	hits := []models.CatalogHit{
		catalogHit("github.com/user/first", "first", "v1"),
		catalogHit("github.com/user/member", "member", "v1"),
		catalogHit("github.com/user/third", "third", "v1"),
		catalogHit("github.com/user/fourth", "fourth", "v1"),
	}
	collections := []domain.Collection{{
		Arrows: []domain.CollectionArrow{
			{Namespace: domain.Namespace("github.com/user/member@v1")},
		},
	}}
	uc := newSearch(hits, nil, nil, nil, collections, nil)

	got, err := uc.Search(context.Background(), models.SearchQuery{Text: "thing"})
	require.NoError(t, err)
	require.Len(t, got, 4)
	assert.Equal(t, domain.Namespace("github.com/user/member"), got[0].Namespace)
}

func TestSearch_NoMatchesReturnsEmptySliceNotError(t *testing.T) {
	uc := newSearch(nil, nil, nil, nil, nil, nil)

	got, err := uc.Search(context.Background(), models.SearchQuery{Text: "nothing"})
	require.NoError(t, err)
	assert.NotNil(t, got)
	assert.Empty(t, got)
}

func TestSearch_CatalogErrorPropagates(t *testing.T) {
	uc := newSearch(nil, errors.New("catalog down"), nil, nil, nil, nil)

	_, err := uc.Search(context.Background(), models.SearchQuery{Text: "pkg"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "catalog down")
}

func TestSearch_VaultErrorPropagates(t *testing.T) {
	uc := newSearch(nil, nil, nil, errors.New("vault down"), nil, nil)

	_, err := uc.Search(context.Background(), models.SearchQuery{Text: "pkg"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vault down")
}

func TestSearch_CollectionListErrorDoesNotFailTheSearch(t *testing.T) {
	hits := []models.CatalogHit{catalogHit("github.com/user/pkg", "pkg", "v1.0.0")}
	uc := newSearch(hits, nil, nil, nil, nil, errors.New("collections down"))

	got, err := uc.Search(context.Background(), models.SearchQuery{Text: "pkg"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, models.ProvenanceInstalled, got[0].Provenance)
}

func TestSearch_RespectsLimitAfterRanking(t *testing.T) {
	hits := []models.CatalogHit{
		catalogHit("github.com/user/a", "a", "v1"),
		catalogHit("github.com/user/b", "b", "v1"),
	}
	rows := []vault.IndexRow{
		vaultRow("github.com/user/c", "v1", "c", 400000),
		vaultRow("github.com/user/d", "v1", "d", 0),
	}
	uc := newSearch(hits, nil, rows, nil, nil, nil)

	got, err := uc.Search(context.Background(), models.SearchQuery{Text: "letter", Limit: 2})
	require.NoError(t, err)
	require.Len(t, got, 2)
	// Each set is normalised on its own, so "a" and "c" both lead their source;
	// the star boost then puts the vault row first. Truncation happens after.
	assert.Equal(t, domain.Namespace("github.com/user/c"), got[0].Namespace)
	assert.Equal(t, domain.Namespace("github.com/user/a"), got[1].Namespace)
}

func TestSearch_ExactNameMatchOutranksEarlierResult(t *testing.T) {
	hits := []models.CatalogHit{
		catalogHit("github.com/user/kafka-ui", "kafka-ui", "v1"),
		catalogHit("github.com/user/kafka", "kafka", "v1"),
		catalogHit("github.com/user/kafkacat", "kafkacat", "v1"),
	}
	uc := newSearch(hits, nil, nil, nil, nil, nil)

	got, err := uc.Search(context.Background(), models.SearchQuery{Text: " kafka "})
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, domain.Namespace("github.com/user/kafka"), got[0].Namespace)
}

func TestSearch_PassesTrimmedTextAndFiltersToBothStores(t *testing.T) {
	var catalogQuery models.SearchQuery
	arrows := &ucmocks.MockArrow{
		SearchFn: func(_ context.Context, q models.SearchQuery) ([]models.CatalogHit, error) {
			catalogQuery = q
			return nil, nil
		},
	}
	v := &mocks.Vault{}
	uc := NewSearchUsecase(arrows, v, &ucmocks.MockCollection{})

	_, err := uc.Search(context.Background(), models.SearchQuery{
		Text:  "  redis  ",
		OS:    domain.OSLinuxARM64,
		Limit: 0,
	})
	require.NoError(t, err)

	assert.Equal(t, models.SearchQuery{
		Text:  "redis",
		OS:    domain.OSLinuxARM64,
		Limit: defaultSearchLimit,
	}, catalogQuery)
	assert.Equal(t, vault.IndexQuery{
		Text:  "redis",
		OS:    domain.OSLinuxARM64,
		Limit: defaultSearchLimit,
	}, v.SearchArrowsQuery)
}

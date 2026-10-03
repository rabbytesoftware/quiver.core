package usecases

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/recommendation"
	ucmocks "github.com/rabbytesoftware/quiver.core/internal/app/usecases/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
)

func homeEntry(
	ns string,
	inCatalog bool,
	refs ...string,
) recommendation.Entry {
	rows := make([]vault.IndexRow, 0, len(refs))
	for _, ref := range refs {
		rows = append(rows, vault.IndexRow{
			Namespace: domain.Namespace(ns),
			Ref:       ref,
			Meta: vault.IndexMeta{
				Arrow:  domain.ArrowMeta{Name: "Name " + ns, Description: "desc", Tags: []string{"t"}},
				OS:     []domain.OS{domain.OSLinuxAMD64},
				Stars:  1,
				Source: "index",
			},
		})
	}
	return recommendation.Entry{Namespace: domain.Namespace(ns), Stars: 42, Source: "github.com", Rows: rows, InCatalog: inCatalog}
}

func TestHomeUsecase_Home_MapsShelvesAndTheRefreshingFlag(t *testing.T) {
	at := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	rec := &ucmocks.MockRecommendation{
		RefreshingFlag: true,
		HomeResult: []recommendation.Shelf{
			{ID: "popular", Title: "Popular", RefreshedAt: at, Entries: []recommendation.Entry{homeEntry("github.com/a/one", false, "v2", "v1")}},
			{ID: "fresh", Title: "Fresh"},
		},
	}

	home, err := NewHomeUsecase(rec).Home(context.Background())

	require.NoError(t, err)
	assert.True(t, home.Refreshing)
	require.Len(t, home.Shelves, 2)
	assert.Equal(t, "popular", home.Shelves[0].ID)
	assert.Equal(t, "Popular", home.Shelves[0].Title)
	assert.True(t, at.Equal(home.Shelves[0].RefreshedAt))
	assert.True(t, home.Shelves[1].RefreshedAt.IsZero())
	assert.NotNil(t, home.Shelves[1].Arrows)
	assert.Empty(t, home.Shelves[1].Arrows)

	arrow := home.Shelves[0].Arrows[0]
	assert.Equal(t, domain.Namespace("github.com/a/one"), arrow.Namespace)
	assert.Equal(t, "Name github.com/a/one", arrow.Name)
	assert.Equal(t, []string{"v1", "v2"}, arrow.Versions)
	assert.Equal(t, []domain.OS{domain.OSLinuxAMD64}, arrow.CompatibleOS)
	assert.Equal(t, 42, arrow.Stars, "the snapshot's stars win over the index row's")
	assert.Equal(t, "github.com", arrow.Source)
	assert.Equal(t, models.ProvenanceSeen, arrow.Provenance)
	assert.True(t, arrow.Known)
	assert.False(t, arrow.Installed)
}

func TestHomeUsecase_Home_ArrowInTheCatalogIsInstalledWithoutProvenance(t *testing.T) {
	rec := &ucmocks.MockRecommendation{HomeResult: []recommendation.Shelf{
		{ID: "popular", Entries: []recommendation.Entry{homeEntry("github.com/a/one", true, "main")}},
	}}

	home, err := NewHomeUsecase(rec).Home(context.Background())

	require.NoError(t, err)
	arrow := home.Shelves[0].Arrows[0]
	assert.True(t, arrow.Installed)
	assert.Empty(t, arrow.Provenance)
	assert.True(t, arrow.Known)
}

func TestHomeUsecase_Home_RepositoryFailure_IsWrapped(t *testing.T) {
	boom := errors.New("boom")
	rec := &ucmocks.MockRecommendation{HomeErr: boom}

	_, err := NewHomeUsecase(rec).Home(context.Background())

	require.ErrorIs(t, err, boom)
}

func TestHomeUsecase_Refresh_TriggersTheRepository(t *testing.T) {
	rec := &ucmocks.MockRecommendation{}

	NewHomeUsecase(rec).Refresh(context.Background())

	assert.Equal(t, 1, rec.RefreshCalls)
}

func TestContainerNew_HomeIsWiredOnlyWithARecommendation(t *testing.T) {
	base := func() *repositories.Container {
		return &repositories.Container{
			Arrow:      &ucmocks.MockArrow{},
			Runtime:    &ucmocks.MockRuntime{},
			Collection: &ucmocks.MockCollection{},
			Graph:      &ucmocks.MockGraph{},
		}
	}

	without, err := New(base(), nil, nil, nil, nil)
	require.NoError(t, err)
	assert.Nil(t, without.Home)

	repos := base()
	repos.Recommendation = &ucmocks.MockRecommendation{}
	with, err := New(repos, nil, nil, nil, nil)
	require.NoError(t, err)
	assert.NotNil(t, with.Home)
}

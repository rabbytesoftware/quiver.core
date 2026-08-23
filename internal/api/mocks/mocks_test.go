package mocks_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

var (
	ctx     = context.Background()
	testNS  = domain.Namespace("github.com/user/repo")
	errTest = errors.New("test error")
)

func TestArrowService_Add(t *testing.T) {
	m := &mocks.ArrowService{AddErr: errTest}
	assert.Equal(t, errTest, m.Add(ctx, testNS))
}

func TestArrowService_Update(t *testing.T) {
	m := &mocks.ArrowService{UpdateErr: errTest}
	_, err := m.Update(ctx, testNS, models.UpdateOptions{})
	assert.Equal(t, errTest, err)
}

func TestArrowService_Remove(t *testing.T) {
	m := &mocks.ArrowService{RemoveErr: errTest}
	assert.Equal(t, errTest, m.Remove(ctx, testNS))
}

func TestArrowService_List(t *testing.T) {
	want := []models.ArrowListDTO{{Namespace: testNS}}
	m := &mocks.ArrowService{ListResult: want}
	got, err := m.List(ctx, nil)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestArrowService_Get(t *testing.T) {
	want := &domain.Arrow{Namespace: testNS}
	m := &mocks.ArrowService{GetResult: want}
	got, err := m.Get(ctx, testNS)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestArrowService_GetDetail(t *testing.T) {
	want := &models.ArrowDetailDTO{Namespace: testNS}
	m := &mocks.ArrowService{GetDetailResult: want}
	got, err := m.GetDetail(ctx, testNS)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestArrowService_HasDependents(t *testing.T) {
	m := &mocks.ArrowService{HasDependentsResult: true}
	got, err := m.HasDependents(ctx, testNS, testNS)
	require.NoError(t, err)
	assert.True(t, got)
}

func TestRuntimeService_Install(t *testing.T) {
	m := &mocks.RuntimeService{InstallStarted: true, InstallErr: errTest}
	started, err := m.Install(ctx, testNS, nil)
	assert.True(t, started)
	assert.Equal(t, errTest, err)
}

func TestRuntimeService_Uninstall(t *testing.T) {
	m := &mocks.RuntimeService{UninstallErr: errTest}
	assert.Equal(t, errTest, m.Uninstall(ctx, testNS, nil))
}

func TestRuntimeService_Execute(t *testing.T) {
	m := &mocks.RuntimeService{ExecuteErr: errTest}
	assert.Equal(t, errTest, m.Execute(ctx, testNS, "run", nil))
}

func TestRuntimeService_Stop(t *testing.T) {
	m := &mocks.RuntimeService{StopErr: errTest}
	assert.Equal(t, errTest, m.Stop(ctx, testNS))
}

func TestHub_BroadcastArrow(t *testing.T) {
	m := &mocks.Hub{}
	m.BroadcastArrow(domain.Arrow{Namespace: testNS})
	assert.Len(t, m.BroadcastedArrows, 1)
	assert.Equal(t, testNS, m.BroadcastedArrows[0].Namespace)
}

func TestHub_BroadcastArrowRuntime(t *testing.T) {
	m := &mocks.Hub{}
	m.BroadcastArrowRuntime(domainRuntime.ArrowRuntime{Ref: testNS})
	assert.Len(t, m.BroadcastedRuntimes, 1)
}

func TestHub_BroadcastCollection(t *testing.T) {
	m := &mocks.Hub{}
	m.BroadcastCollection(domain.Collection{Namespace: testNS})
	assert.Len(t, m.BroadcastedQuivers, 1)
}

func TestCollectionService_List(t *testing.T) {
	want := []models.CollectionListDTO{{Namespace: testNS}}
	m := &mocks.CollectionService{ListResult: want}
	got, err := m.List(ctx, nil)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestCollectionService_Get(t *testing.T) {
	want := &models.CollectionDetailDTO{Namespace: testNS}
	m := &mocks.CollectionService{GetResult: want}
	got, err := m.Get(ctx, testNS)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestSearchService_Search(t *testing.T) {
	want := []models.SearchResult{{Namespace: testNS}}
	m := &mocks.SearchService{SearchResult: want}

	got, err := m.Search(ctx, models.SearchQuery{Text: "repo"})
	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, models.SearchQuery{Text: "repo"}, m.SearchQuery)
	assert.Equal(t, 1, m.SearchCalls)
}

func TestSearchService_SearchError(t *testing.T) {
	m := &mocks.SearchService{SearchErr: errTest}

	_, err := m.Search(ctx, models.SearchQuery{Text: "repo"})
	assert.Equal(t, errTest, err)
}

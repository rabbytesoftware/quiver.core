package mocks_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/domain/auth"
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
	m := &mocks.RuntimeService{InstallErr: errTest}
	assert.Equal(t, errTest, m.Install(ctx, testNS, nil))
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

func TestAuthService_GeneratePairingCode(t *testing.T) {
	expiresAt := time.Now().Add(5 * time.Minute)
	m := &mocks.AuthService{GenerateCode: "482913", GenerateExpiresAt: expiresAt, GenerateErr: errTest}

	code, got, err := m.GeneratePairingCode(ctx)
	assert.Equal(t, "482913", code)
	assert.Equal(t, expiresAt, got)
	assert.Equal(t, errTest, err)
}

func TestAuthService_Redeem(t *testing.T) {
	m := &mocks.AuthService{RedeemToken: "tok", RedeemErr: errTest}

	token, err := m.Redeem(ctx, "482913", "dev-1", "laptop")
	assert.Equal(t, "tok", token)
	assert.Equal(t, errTest, err)
	assert.Equal(t, "482913", m.RedeemArgs.Code)
	assert.Equal(t, "dev-1", m.RedeemArgs.DeviceID)
	assert.Equal(t, "laptop", m.RedeemArgs.Label)
}

func TestAuthService_Authenticate(t *testing.T) {
	want := auth.Device{ID: "dev-1"}
	m := &mocks.AuthService{AuthenticateResult: want, AuthenticateErr: errTest}

	got, err := m.Authenticate(ctx, "raw-token")
	assert.Equal(t, want, got)
	assert.Equal(t, errTest, err)
	assert.Equal(t, "raw-token", m.AuthenticateToken)
}

func TestAuthService_ListDevices(t *testing.T) {
	want := []auth.Device{{ID: "dev-1"}}
	m := &mocks.AuthService{ListDevicesResult: want, ListDevicesErr: errTest}

	got, err := m.ListDevices(ctx)
	assert.Equal(t, want, got)
	assert.Equal(t, errTest, err)
}

func TestAuthService_RevokeDevice(t *testing.T) {
	m := &mocks.AuthService{RevokeDeviceErr: errTest}

	err := m.RevokeDevice(ctx, "dev-1")
	assert.Equal(t, errTest, err)
	assert.Equal(t, "dev-1", m.RevokeDeviceID)
}

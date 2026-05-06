package mocks_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver/internal/api/mocks"
	"github.com/rabbytesoftware/quiver/internal/app/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
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

func TestHub_BroadcastQuiver(t *testing.T) {
	m := &mocks.Hub{}
	m.BroadcastQuiver(domain.Quiver{Namespace: testNS})
	assert.Len(t, m.BroadcastedQuivers, 1)
}

func TestQuiverService_List(t *testing.T) {
	want := []models.QuiverListDTO{{Namespace: testNS}}
	m := &mocks.QuiverService{ListResult: want}
	got, err := m.List(ctx, nil)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestQuiverService_Get(t *testing.T) {
	want := &models.QuiverDetailDTO{Namespace: testNS}
	m := &mocks.QuiverService{GetResult: want}
	got, err := m.Get(ctx, testNS)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

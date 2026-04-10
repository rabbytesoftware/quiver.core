package mocks_test

import (
	"context"
	"errors"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/api/mocks"
	"github.com/rabbytesoftware/quiver/internal/app/arrow"
	appquiver "github.com/rabbytesoftware/quiver/internal/app/quiver"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	ctx     = context.Background()
	testNS  = domain.Namespace("github.com/user/repo")
	testErr = errors.New("test error")
)

func TestArrowService_Add(t *testing.T) {
	m := &mocks.ArrowService{AddErr: testErr}
	assert.Equal(t, testErr, m.Add(ctx, testNS))
}

func TestArrowService_Update(t *testing.T) {
	m := &mocks.ArrowService{UpdateErr: testErr}
	assert.Equal(t, testErr, m.Update(ctx, testNS))
}

func TestArrowService_Remove(t *testing.T) {
	m := &mocks.ArrowService{RemoveErr: testErr}
	assert.Equal(t, testErr, m.Remove(ctx, testNS))
}

func TestArrowService_List(t *testing.T) {
	want := []arrow.ArrowListDTO{{Namespace: testNS}}
	m := &mocks.ArrowService{ListResult: want}
	got, err := m.List(ctx)
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
	want := &arrow.ArrowDetailDTO{Namespace: testNS}
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

func TestArrowService_Install(t *testing.T) {
	m := &mocks.ArrowService{InstallErr: testErr}
	assert.Equal(t, testErr, m.Install(ctx, testNS, nil))
}

func TestArrowService_Uninstall(t *testing.T) {
	m := &mocks.ArrowService{UninstallErr: testErr}
	assert.Equal(t, testErr, m.Uninstall(ctx, testNS, nil))
}

func TestArrowService_BeginExecution(t *testing.T) {
	m := &mocks.ArrowService{BeginExecutionErr: testErr}
	assert.Equal(t, testErr, m.BeginExecution(ctx, testNS, "run", nil))
}

func TestArrowService_Stop(t *testing.T) {
	m := &mocks.ArrowService{StopErr: testErr}
	assert.Equal(t, testErr, m.Stop(ctx, testNS))
}

func TestHub_BroadcastArrow(t *testing.T) {
	m := &mocks.Hub{}
	m.BroadcastArrow(domain.Arrow{Namespace: testNS})
	assert.Len(t, m.BroadcastedArrows, 1)
	assert.Equal(t, testNS, m.BroadcastedArrows[0].Namespace)
}

func TestHub_BroadcastArrowRuntime(t *testing.T) {
	m := &mocks.Hub{}
	m.BroadcastArrowRuntime(domainRuntime.ArrowRuntime{Namespace: testNS})
	assert.Len(t, m.BroadcastedRuntimes, 1)
}

func TestHub_BroadcastQuiver(t *testing.T) {
	m := &mocks.Hub{}
	m.BroadcastQuiver(domain.Quiver{Namespace: testNS})
	assert.Len(t, m.BroadcastedQuivers, 1)
}

func TestQuiverService_Add(t *testing.T) {
	m := &mocks.QuiverService{AddErr: testErr}
	assert.Equal(t, testErr, m.Add(ctx, testNS))
}

func TestQuiverService_Update(t *testing.T) {
	m := &mocks.QuiverService{UpdateErr: testErr}
	assert.Equal(t, testErr, m.Update(ctx, testNS))
}

func TestQuiverService_Remove(t *testing.T) {
	m := &mocks.QuiverService{RemoveErr: testErr}
	assert.Equal(t, testErr, m.Remove(ctx, testNS))
}

func TestQuiverService_List(t *testing.T) {
	want := []appquiver.QuiverListDTO{{Namespace: testNS}}
	m := &mocks.QuiverService{ListResult: want}
	got, err := m.List(ctx)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestQuiverService_Get(t *testing.T) {
	want := &appquiver.QuiverDetailDTO{Namespace: testNS}
	m := &mocks.QuiverService{GetResult: want}
	got, err := m.Get(ctx, testNS)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

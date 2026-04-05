package projections

import (
	"context"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/stretchr/testify/assert"
)

type mockStopWizard struct {
	cancelled []domain.Namespace
}

func (m *mockStopWizard) Cancel(ns domain.Namespace) {
	m.cancelled = append(m.cancelled, ns)
}

func TestStopCoordinator_OnMarkStopping_CallsWizardCancel(t *testing.T) {
	wiz := &mockStopWizard{}
	handler := StopCoordinator(wiz)

	ns := domain.Namespace("github.com/org/repo")
	rt := domainRuntime.ArrowRuntime{
		Namespace: ns,
		State:     domain.ArrowStateStopping,
	}
	evt := asynxModels.Event[domainRuntime.ArrowRuntime]{
		EventName: "runtime.mark_stopping",
		Aggregate: rt,
	}

	handler(context.Background(), evt)

	assert.Len(t, wiz.cancelled, 1)
	assert.Equal(t, ns, wiz.cancelled[0])
}

func TestStopCoordinator_OnOtherEvent_DoesNotCallCancel(t *testing.T) {
	wiz := &mockStopWizard{}
	handler := StopCoordinator(wiz)

	ns := domain.Namespace("github.com/org/repo")
	rt := domainRuntime.ArrowRuntime{
		Namespace: ns,
		State:     domain.ArrowStateRunning,
	}
	evt := asynxModels.Event[domainRuntime.ArrowRuntime]{
		EventName: "runtime.begun",
		Aggregate: rt,
	}

	handler(context.Background(), evt)

	assert.Empty(t, wiz.cancelled)
}

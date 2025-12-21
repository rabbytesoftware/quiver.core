package registry

import (
	"testing"

	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testEvent struct {
	domain.BaseEvent
}

func (e *testEvent) GetEventType() string {
	return "test.Event"
}

func TestNewRegistry(t *testing.T) {
	reg := NewRegistry()
	assert.NotNil(t, reg)
	assert.NotNil(t, reg.events)
}

func TestRegistry_RegisterEvent(t *testing.T) {
	reg := NewRegistry()
	event := &testEvent{
		BaseEvent: domain.NewBaseEvent("agg1", "test", "test.Event"),
	}

	reg.RegisterEvent(event)

	typ, err := reg.GetEventType("test.Event")
	require.NoError(t, err)
	assert.NotNil(t, typ)
}

func TestRegistry_GetEventType_NotRegistered(t *testing.T) {
	reg := NewRegistry()

	_, err := reg.GetEventType("nonexistent.Event")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")
}


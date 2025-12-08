package eventsourcing_test

import (
	"context"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing"
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/memory"
	"github.com/stretchr/testify/assert"
)

type TestWizardEvent interface {
	eventsourcing.Event
	GetWizardID() string
}

type TestWizardStartedEvent struct {
	eventsourcing.BaseEvent
	WizardID  string `json:"wizard_id"`
	Method    string `json:"method"`
	ArrowName string `json:"arrow_name"`
	Platform  string `json:"platform"`
}

func (e *TestWizardStartedEvent) GetWizardID() string {
	return e.WizardID
}

func NewTestWizardStarted(arrowID, wizardID, method, arrowName, platform string) *TestWizardStartedEvent {
	return &TestWizardStartedEvent{
		BaseEvent: eventsourcing.NewBaseEvent(arrowID, "arrow", "test.wizard.started"),
		WizardID:  wizardID,
		Method:    method,
		ArrowName: arrowName,
		Platform:  platform,
	}
}

type TestActionCompletedEvent struct {
	eventsourcing.BaseEvent
	WizardID    string `json:"wizard_id"`
	ActionIndex int    `json:"action_index"`
	ActionName  string `json:"action_name"`
	DurationMs  int64  `json:"duration_ms"`
}

func (e *TestActionCompletedEvent) GetWizardID() string {
	return e.WizardID
}

func NewTestActionCompleted(arrowID, wizardID string, actionIndex int, actionName string, duration int64) *TestActionCompletedEvent {
	return &TestActionCompletedEvent{
		BaseEvent:   eventsourcing.NewBaseEvent(arrowID, "arrow", "test.action.completed"),
		WizardID:    wizardID,
		ActionIndex: actionIndex,
		ActionName:  actionName,
		DurationMs:  duration,
	}
}

type TestWizardCompletedEvent struct {
	eventsourcing.BaseEvent
	WizardID         string `json:"wizard_id"`
	Method           string `json:"method"`
	TotalDurationMs  int64  `json:"total_duration_ms"`
	ActionsCompleted int    `json:"actions_completed"`
}

func (e *TestWizardCompletedEvent) GetWizardID() string {
	return e.WizardID
}

func NewTestWizardCompleted(arrowID, wizardID, method string, duration int64, actions int) *TestWizardCompletedEvent {
	return &TestWizardCompletedEvent{
		BaseEvent:        eventsourcing.NewBaseEvent(arrowID, "arrow", "test.wizard.completed"),
		WizardID:         wizardID,
		Method:           method,
		TotalDurationMs:  duration,
		ActionsCompleted: actions,
	}
}

type TestWizardFailedEvent struct {
	eventsourcing.BaseEvent
	WizardID   string `json:"wizard_id"`
	Method     string `json:"method"`
	Error      string `json:"error"`
	LastAction int    `json:"last_action"`
}

func (e *TestWizardFailedEvent) GetWizardID() string {
	return e.WizardID
}

func NewTestWizardFailed(arrowID, wizardID, method, errorMsg string, lastAction int) *TestWizardFailedEvent {
	return &TestWizardFailedEvent{
		BaseEvent:  eventsourcing.NewBaseEvent(arrowID, "arrow", "test.wizard.failed"),
		WizardID:   wizardID,
		Method:     method,
		Error:      errorMsg,
		LastAction: lastAction,
	}
}

func TestBuilderConfiguration(t *testing.T) {
	ev := eventsourcing.NewBuilder[TestWizardEvent]().
		WithStore(memory.NewEventStore[TestWizardEvent]()).
		WithBus(memory.NewEventBus()).
		WithRetentionDays(30).
		WithAutoReplay(false).
		WithEventTypes([]eventsourcing.EventTypeRegistration[TestWizardEvent]{
			{EventType: "test.wizard.started", Factory: func() TestWizardEvent { return &TestWizardStartedEvent{} }},
			{EventType: "test.wizard.completed", Factory: func() TestWizardEvent { return &TestWizardCompletedEvent{} }},
			{EventType: "test.action.completed", Factory: func() TestWizardEvent { return &TestActionCompletedEvent{} }},
			{EventType: "test.wizard.failed", Factory: func() TestWizardEvent { return &TestWizardFailedEvent{} }},
		}).
		Build()

	assert.NotNil(t, ev)
	assert.NotNil(t, ev.Store())
	assert.NotNil(t, ev.Bus())
	assert.NotNil(t, ev.Registry())
}

func TestSingleEmitMethod(t *testing.T) {
	ctx := context.Background()
	arrowID := "arrow_test_123"
	wizardID := "wizard_test_456"

	ev := eventsourcing.NewBuilder[TestWizardEvent]().
		WithStore(memory.NewEventStore[TestWizardEvent]()).
		WithBus(memory.NewEventBus()).
		WithEventTypes([]eventsourcing.EventTypeRegistration[TestWizardEvent]{
			{EventType: "test.wizard.started", Factory: func() TestWizardEvent { return &TestWizardStartedEvent{} }},
		}).
		Build()

	event := NewTestWizardStarted(arrowID, wizardID, "install", "Test Arrow", "darwin/arm64")

	err := ev.Emit(ctx, event)
	assert.NoError(t, err)

	count, err := ev.Store().CountByAggregate(ctx, arrowID)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), count)
}

func TestZeroTypeAssertions(t *testing.T) {
	ctx := context.Background()
	arrowID := "arrow_cs2_123"
	wizardID := "wizard_xyz_789"

	ev := eventsourcing.NewBuilder[TestWizardEvent]().
		WithStore(memory.NewEventStore[TestWizardEvent]()).
		WithBus(memory.NewEventBus()).
		WithEventTypes([]eventsourcing.EventTypeRegistration[TestWizardEvent]{
			{EventType: "test.wizard.started", Factory: func() TestWizardEvent { return &TestWizardStartedEvent{} }},
			{EventType: "test.action.completed", Factory: func() TestWizardEvent { return &TestActionCompletedEvent{} }},
			{EventType: "test.wizard.completed", Factory: func() TestWizardEvent { return &TestWizardCompletedEvent{} }},
		}).
		Build()

	ev.Emit(ctx, NewTestWizardStarted(arrowID, wizardID, "install", "CS2 Server", "darwin/arm64"))
	ev.Emit(ctx, NewTestActionCompleted(arrowID, wizardID, 0, "Download", 2000))
	ev.Emit(ctx, NewTestActionCompleted(arrowID, wizardID, 1, "Install", 5000))
	ev.Emit(ctx, NewTestWizardCompleted(arrowID, wizardID, "install", 7000, 2))

	stream := ev.GetEvents(ctx, arrowID)
	assert.Equal(t, 4, stream.Count())

	completed := eventsourcing.FindLast[TestWizardCompletedEvent](stream, "test.wizard.completed")
	assert.NotNil(t, completed)
	assert.Equal(t, "install", completed.Method)
	assert.Equal(t, int64(7000), completed.TotalDurationMs)
	assert.Equal(t, 2, completed.ActionsCompleted)
}

func TestMultiAggregateSupport(t *testing.T) {
	ctx := context.Background()
	arrowCS2 := "arrow_cs2_123"
	arrowMinecraft := "arrow_minecraft_456"
	wizardID1 := "wizard_1"
	wizardID2 := "wizard_2"

	ev := eventsourcing.NewBuilder[TestWizardEvent]().
		WithStore(memory.NewEventStore[TestWizardEvent]()).
		WithBus(memory.NewEventBus()).
		WithEventTypes([]eventsourcing.EventTypeRegistration[TestWizardEvent]{
			{EventType: "test.wizard.started", Factory: func() TestWizardEvent { return &TestWizardStartedEvent{} }},
			{EventType: "test.wizard.completed", Factory: func() TestWizardEvent { return &TestWizardCompletedEvent{} }},
		}).
		Build()

	ev.Emit(ctx, NewTestWizardStarted(arrowCS2, wizardID1, "install", "CS2", "darwin"))
	ev.Emit(ctx, NewTestWizardCompleted(arrowCS2, wizardID1, "install", 5000, 3))

	ev.Emit(ctx, NewTestWizardStarted(arrowMinecraft, wizardID2, "install", "Minecraft", "darwin"))
	ev.Emit(ctx, NewTestWizardCompleted(arrowMinecraft, wizardID2, "install", 3000, 2))

	cs2Stream := ev.GetEvents(ctx, arrowCS2)
	assert.Equal(t, 2, cs2Stream.Count())

	minecraftStream := ev.GetEvents(ctx, arrowMinecraft)
	assert.Equal(t, 2, minecraftStream.Count())

	cs2Completed := eventsourcing.FindLast[TestWizardCompletedEvent](cs2Stream, "test.wizard.completed")
	assert.NotNil(t, cs2Completed)
	assert.Equal(t, "install", cs2Completed.Method)

	minecraftCompleted := eventsourcing.FindLast[TestWizardCompletedEvent](minecraftStream, "test.wizard.completed")
	assert.NotNil(t, minecraftCompleted)
	assert.Equal(t, "install", minecraftCompleted.Method)
}

func TestStateReconstruction(t *testing.T) {
	ctx := context.Background()
	arrowID := "arrow_test_789"
	wizardID := "wizard_abc"

	ev := eventsourcing.NewBuilder[TestWizardEvent]().
		WithStore(memory.NewEventStore[TestWizardEvent]()).
		WithBus(memory.NewEventBus()).
		WithEventTypes([]eventsourcing.EventTypeRegistration[TestWizardEvent]{
			{EventType: "test.wizard.started", Factory: func() TestWizardEvent { return &TestWizardStartedEvent{} }},
			{EventType: "test.action.completed", Factory: func() TestWizardEvent { return &TestActionCompletedEvent{} }},
			{EventType: "test.wizard.completed", Factory: func() TestWizardEvent { return &TestWizardCompletedEvent{} }},
		}).
		Build()

	ev.Emit(ctx, NewTestWizardStarted(arrowID, wizardID, "install", "Test", "linux"))
	ev.Emit(ctx, NewTestActionCompleted(arrowID, wizardID, 0, "Download", 1000))
	ev.Emit(ctx, NewTestActionCompleted(arrowID, wizardID, 1, "Extract", 2000))
	ev.Emit(ctx, NewTestActionCompleted(arrowID, wizardID, 2, "Install", 3000))
	ev.Emit(ctx, NewTestWizardCompleted(arrowID, wizardID, "install", 6000, 3))

	stream := ev.GetEvents(ctx, arrowID)

	status := "unknown"
	lastMethod := ""
	actionsCompleted := 0

	for _, event := range stream.All() {
		switch e := event.(type) {
		case *TestWizardStartedEvent:
			status = "installing"
			lastMethod = e.Method
		case *TestActionCompletedEvent:
			actionsCompleted++
		case *TestWizardCompletedEvent:
			status = "installed"
			lastMethod = e.Method
			actionsCompleted = e.ActionsCompleted
		}
	}

	assert.Equal(t, "installed", status)
	assert.Equal(t, "install", lastMethod)
	assert.Equal(t, 3, actionsCompleted)
}

func TestResumeCapability(t *testing.T) {
	ctx := context.Background()
	arrowID := "arrow_failed_123"
	wizardID := "wizard_fail"

	ev := eventsourcing.NewBuilder[TestWizardEvent]().
		WithStore(memory.NewEventStore[TestWizardEvent]()).
		WithBus(memory.NewEventBus()).
		WithEventTypes([]eventsourcing.EventTypeRegistration[TestWizardEvent]{
			{EventType: "test.wizard.started", Factory: func() TestWizardEvent { return &TestWizardStartedEvent{} }},
			{EventType: "test.action.completed", Factory: func() TestWizardEvent { return &TestActionCompletedEvent{} }},
			{EventType: "test.wizard.failed", Factory: func() TestWizardEvent { return &TestWizardFailedEvent{} }},
		}).
		Build()

	ev.Emit(ctx, NewTestWizardStarted(arrowID, wizardID, "install", "Failed Arrow", "darwin"))
	ev.Emit(ctx, NewTestActionCompleted(arrowID, wizardID, 0, "Download", 1000))
	ev.Emit(ctx, NewTestActionCompleted(arrowID, wizardID, 1, "Extract", 2000))
	ev.Emit(ctx, NewTestWizardFailed(arrowID, wizardID, "install", "Network timeout", 1))

	stream := ev.GetEvents(ctx, arrowID)

	failed := eventsourcing.FindLast[TestWizardFailedEvent](stream, "test.wizard.failed")
	assert.NotNil(t, failed)
	assert.Equal(t, 1, failed.LastAction)
	assert.Equal(t, "Network timeout", failed.Error)

	resumeFromAction := failed.LastAction + 1
	assert.Equal(t, 2, resumeFromAction)
}

func TestEventBusPubSub(t *testing.T) {
	ctx := context.Background()
	arrowID := "arrow_pubsub_123"
	wizardID := "wizard_pubsub"

	ev := eventsourcing.NewBuilder[TestWizardEvent]().
		WithStore(memory.NewEventStore[TestWizardEvent]()).
		WithBus(memory.NewEventBus()).
		WithEventTypes([]eventsourcing.EventTypeRegistration[TestWizardEvent]{
			{EventType: "test.wizard.completed", Factory: func() TestWizardEvent { return &TestWizardCompletedEvent{} }},
		}).
		Build()

	received := make(chan eventsourcing.Event, 1)
	ev.Bus().Subscribe("test.wizard.completed", func(ctx context.Context, event eventsourcing.Event) error {
		received <- event
		return nil
	})

	event := NewTestWizardCompleted(arrowID, wizardID, "install", 5000, 3)
	err := ev.Emit(ctx, event)
	assert.NoError(t, err)

	select {
	case receivedEvent := <-received:
		assert.Equal(t, "test.wizard.completed", receivedEvent.GetEventType())
		assert.Equal(t, arrowID, receivedEvent.GetAggregateID())
	case <-time.After(1 * time.Second):
		t.Fatal("Event not received via event bus")
	}
}

func TestFilterTyped(t *testing.T) {
	ctx := context.Background()
	arrowID := "arrow_filter_123"
	wizardID := "wizard_filter"

	ev := eventsourcing.NewBuilder[TestWizardEvent]().
		WithStore(memory.NewEventStore[TestWizardEvent]()).
		WithBus(memory.NewEventBus()).
		WithEventTypes([]eventsourcing.EventTypeRegistration[TestWizardEvent]{
			{EventType: "test.wizard.started", Factory: func() TestWizardEvent { return &TestWizardStartedEvent{} }},
			{EventType: "test.action.completed", Factory: func() TestWizardEvent { return &TestActionCompletedEvent{} }},
			{EventType: "test.wizard.completed", Factory: func() TestWizardEvent { return &TestWizardCompletedEvent{} }},
		}).
		Build()

	ev.Emit(ctx, NewTestWizardStarted(arrowID, wizardID, "install", "Test", "darwin"))
	ev.Emit(ctx, NewTestActionCompleted(arrowID, wizardID, 0, "Action 1", 1000))
	ev.Emit(ctx, NewTestActionCompleted(arrowID, wizardID, 1, "Action 2", 2000))
	ev.Emit(ctx, NewTestActionCompleted(arrowID, wizardID, 2, "Action 3", 3000))
	ev.Emit(ctx, NewTestWizardCompleted(arrowID, wizardID, "install", 6000, 3))

	stream := ev.GetEvents(ctx, arrowID)

	actionEvents := eventsourcing.Filter[TestActionCompletedEvent](stream, "test.action.completed")
	assert.Len(t, actionEvents, 3)
	assert.Equal(t, 0, actionEvents[0].ActionIndex)
	assert.Equal(t, 1, actionEvents[1].ActionIndex)
	assert.Equal(t, 2, actionEvents[2].ActionIndex)
}

func TestProjectionWithVersionTracking(t *testing.T) {
	ctx := context.Background()
	arrowID := "arrow_projection_123"
	wizardID := "wizard_projection"

	ev := eventsourcing.NewBuilder[TestWizardEvent]().
		WithStore(memory.NewEventStore[TestWizardEvent]()).
		WithBus(memory.NewEventBus()).
		WithEventTypes([]eventsourcing.EventTypeRegistration[TestWizardEvent]{
			{EventType: "test.wizard.started", Factory: func() TestWizardEvent { return &TestWizardStartedEvent{} }},
			{EventType: "test.action.completed", Factory: func() TestWizardEvent { return &TestActionCompletedEvent{} }},
			{EventType: "test.wizard.completed", Factory: func() TestWizardEvent { return &TestWizardCompletedEvent{} }},
		}).
		Build()

	ev.Emit(ctx, NewTestWizardStarted(arrowID, wizardID, "install", "Test", "darwin"))
	ev.Emit(ctx, NewTestActionCompleted(arrowID, wizardID, 0, "Action 1", 1000))
	ev.Emit(ctx, NewTestActionCompleted(arrowID, wizardID, 1, "Action 2", 2000))
	ev.Emit(ctx, NewTestWizardCompleted(arrowID, wizardID, "install", 3000, 2))

	stream := ev.GetEvents(ctx, arrowID)

	eventsProcessed := int64(stream.Count())
	lastVersion := int64(0)
	for _, event := range stream.All() {
		if event.GetVersion() > lastVersion {
			lastVersion = event.GetVersion()
		}
	}

	assert.Equal(t, int64(4), eventsProcessed)

	eventCount, err := ev.Store().CountByAggregate(ctx, arrowID)
	assert.NoError(t, err)
	assert.Equal(t, eventsProcessed, eventCount)

	isStale := eventsProcessed < eventCount
	assert.False(t, isStale)
}

package models

import (
	"testing"

	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
)

func TestExecution_EmitAndDrain(t *testing.T) {
	exec := NewExecution()

	exec.Emit(Event{Kind: EventKindStepStarted, StepIndex: 0})
	exec.Finish(domainRuntime.ExecutionOutcomeSuccess)

	var events []Event
	for e := range exec.Events() {
		events = append(events, e)
	}

	// Expect StepStarted + EventKindEnded (emitted by Finish)
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	if events[0].Kind != EventKindStepStarted {
		t.Errorf("events[0].Kind = %v, want %v", events[0].Kind, EventKindStepStarted)
	}
	if events[1].Kind != EventKindEnded {
		t.Errorf("events[1].Kind = %v, want %v", events[1].Kind, EventKindEnded)
	}
	if events[1].Outcome != domainRuntime.ExecutionOutcomeSuccess {
		t.Errorf("events[1].Outcome = %v, want %v", events[1].Outcome, domainRuntime.ExecutionOutcomeSuccess)
	}

	select {
	case <-exec.Done():
	default:
		t.Error("Done() should be closed after Finish()")
	}

	if exec.Outcome() != domainRuntime.ExecutionOutcomeSuccess {
		t.Errorf("Outcome() = %v, want %v", exec.Outcome(), domainRuntime.ExecutionOutcomeSuccess)
	}
}

func TestExecution_EmitDrop_WhenFull(t *testing.T) {
	exec := NewExecution()

	// Fill all 16 slots; EventKindEnded from Finish will be dropped (slot 17).
	for i := range 16 {
		exec.Emit(Event{Kind: EventKindStepStarted, StepIndex: i})
	}
	exec.Finish(domainRuntime.ExecutionOutcomeSuccess)

	count := 0
	for range exec.Events() {
		count++
	}
	if count != 16 {
		t.Errorf("got %d events, want 16 (EventKindEnded dropped when full)", count)
	}

	// Outcome() is always authoritative even when EventKindEnded is dropped.
	if exec.Outcome() != domainRuntime.ExecutionOutcomeSuccess {
		t.Errorf("Outcome() = %v, want %v", exec.Outcome(), domainRuntime.ExecutionOutcomeSuccess)
	}
}

func TestExecution_OutcomeBeforeFinish(t *testing.T) {
	exec := NewExecution()

	if exec.Outcome() != "" {
		t.Errorf("Outcome() before Finish = %q, want empty", exec.Outcome())
	}
}

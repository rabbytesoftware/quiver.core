package runtime

import (
	"encoding/json"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
)

func TestStepProgress_JSONRoundTrip_WithStep(t *testing.T) {
	s := step.NewRunStep("do thing", "echo hi", false, "5s", true)
	original := StepProgress{
		Index:  0,
		Status: StepStatusRunning,
		Step:   s,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}

	var got StepProgress
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("UnmarshalJSON() error = %v", err)
	}

	if got.Index != original.Index {
		t.Errorf("Index = %d, want %d", got.Index, original.Index)
	}
	if got.Status != original.Status {
		t.Errorf("Status = %q, want %q", got.Status, original.Status)
	}
	if got.Step == nil {
		t.Fatal("Step is nil after round-trip")
	}
	if got.Step.Type() != step.StepTypeRun {
		t.Errorf("Step.Type() = %q, want %q", got.Step.Type(), step.StepTypeRun)
	}
}

func TestStepProgress_JSONRoundTrip_WithError(t *testing.T) {
	errMsg := "something went wrong"
	original := StepProgress{
		Index:  2,
		Status: StepStatusFailed,
		Error:  &errMsg,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}

	var got StepProgress
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("UnmarshalJSON() error = %v", err)
	}

	if got.Error == nil {
		t.Fatal("Error is nil after round-trip")
	}
	if *got.Error != errMsg {
		t.Errorf("Error = %q, want %q", *got.Error, errMsg)
	}
	if got.Step != nil {
		t.Errorf("Step should be nil when no step provided, got %v", got.Step)
	}
}

func TestStepProgress_JSONRoundTrip_NilStep(t *testing.T) {
	original := StepProgress{
		Index:  1,
		Status: StepStatusPending,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}

	var got StepProgress
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("UnmarshalJSON() error = %v", err)
	}

	if got.Step != nil {
		t.Errorf("Step = %v, want nil", got.Step)
	}
	if got.Index != 1 {
		t.Errorf("Index = %d, want 1", got.Index)
	}
}

func TestStepProgress_UnmarshalJSON_InvalidJSON(t *testing.T) {
	var sp StepProgress
	err := json.Unmarshal([]byte(`not valid json`), &sp)
	if err == nil {
		t.Error("expected error on invalid JSON, got nil")
	}
}

package runtime

import (
	"encoding/json"
	"fmt"

	"github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
)

type StepStatus string

const (
	StepStatusPending   StepStatus = "pending"
	StepStatusRunning   StepStatus = "running"
	StepStatusCompleted StepStatus = "completed"
	StepStatusFailed    StepStatus = "failed"
)

type StepProgress struct {
	Index  int
	Status StepStatus
	Error  *string
	Step   step.Step
}

type stepProgressJSON struct {
	Index  int           `json:"Index"`
	Status StepStatus    `json:"Status"`
	Error  *string       `json:"Error"`
	Step   step.StepList `json:"Step"`
}

func (sp StepProgress) MarshalJSON() ([]byte, error) {
	var steps step.StepList
	if sp.Step != nil {
		steps = step.StepList{sp.Step}
	}
	return json.Marshal(stepProgressJSON{
		Index:  sp.Index,
		Status: sp.Status,
		Error:  sp.Error,
		Step:   steps,
	})
}

func (sp *StepProgress) UnmarshalJSON(data []byte) error {
	var raw stepProgressJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("StepProgress: %w", err)
	}
	sp.Index = raw.Index
	sp.Status = raw.Status
	sp.Error = raw.Error
	if len(raw.Step) > 0 {
		sp.Step = raw.Step[0]
	}
	return nil
}

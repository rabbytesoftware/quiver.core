package runtime

import "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"

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

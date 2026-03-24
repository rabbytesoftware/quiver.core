package runtime

type ExecutionOutcome string

const (
	ExecutionOutcomeSuccess   ExecutionOutcome = "success"
	ExecutionOutcomeFailed    ExecutionOutcome = "failed"
	ExecutionOutcomeCancelled ExecutionOutcome = "cancelled"
)

type Execution struct {
	Method    string
	Steps     []StepProgress
	Variables map[string]string
}

type Return struct {
	Method    string
	Outcome   ExecutionOutcome
	Steps     []StepProgress
	Variables map[string]string
}

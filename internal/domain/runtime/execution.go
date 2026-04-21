package runtime

type ExecutionOutcome string

const (
	ExecutionOutcomeSuccess   ExecutionOutcome = "success"
	ExecutionOutcomeFailed    ExecutionOutcome = "failed"
	ExecutionOutcomeCancelled ExecutionOutcome = "cancelled"
)

type Execution struct {
	Method    string            `json:"method"`
	Steps     []StepProgress    `json:"steps"`
	Variables map[string]string `json:"variables"`
}

type Return struct {
	Method    string            `json:"method"`
	Outcome   ExecutionOutcome  `json:"outcome"`
	Steps     []StepProgress    `json:"steps"`
	Variables map[string]string `json:"variables"`
}

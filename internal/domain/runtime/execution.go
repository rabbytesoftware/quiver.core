package runtime

type ExecutionOutcome string

const (
	ExecutionOutcomeSuccess   ExecutionOutcome = "success"
	ExecutionOutcomeFailed    ExecutionOutcome = "failed"
	ExecutionOutcomeCancelled ExecutionOutcome = "cancelled"
)

// Execution is the run the arrow is currently performing. ID identifies that
// run: a stop or an update may take over an arrow while an earlier run is still
// alive, and progress reported by the run that was taken over must not land on
// the one that replaced it.
type Execution struct {
	ID        string            `json:"id"`
	Method    string            `json:"method"`
	Steps     []StepProgress    `json:"steps"`
	Variables map[string]string `json:"variables"`
	PID       int               `json:"pid,omitempty"`
	WorkDir   string            `json:"workDir,omitempty"`
}

type Return struct {
	Method    string            `json:"method"`
	Outcome   ExecutionOutcome  `json:"outcome"`
	Steps     []StepProgress    `json:"steps"`
	Variables map[string]string `json:"variables"`
}

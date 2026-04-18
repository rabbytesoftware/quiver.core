package step

type StepType string

const (
	StepTypeRun          StepType = "run"
	StepTypeFetch        StepType = "fetch"
	StepTypeSignal       StepType = "signal"
	StepTypeDependencies StepType = "dependencies"
)

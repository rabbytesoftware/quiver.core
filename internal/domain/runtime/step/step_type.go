package step

type StepType string

const (
	StepTypeRun          StepType = "run"
	StepTypeFetch        StepType = "fetch"
	StepTypeExtract      StepType = "extract"
	StepTypeSignal       StepType = "signal"
	StepTypeDependencies StepType = "dependencies"
)

package step

type StepType string

const (
	StepTypeRun          StepType = "run"
	StepTypeFetch        StepType = "fetch"
	StepTypeExtract      StepType = "extract"
	StepTypePortable     StepType = "portable"
	StepTypeSignal       StepType = "signal"
	StepTypeDependencies StepType = "dependencies"
)

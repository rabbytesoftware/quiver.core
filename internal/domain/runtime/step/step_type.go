package step

type StepType string

const (
	StepTypeRun          StepType = "run"
	StepTypeFetch        StepType = "fetch"
	StepTypeExtract      StepType = "extract"
	StepTypePortable     StepType = "portable"
	StepTypeSignal       StepType = "signal"
	StepTypeDependencies StepType = "dependencies"
	StepTypeExpose       StepType = "expose"
	StepTypeUnexpose     StepType = "unexpose"
	StepTypeUI           StepType = "ui"
)

package domain

import "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"

type TargetLifecycle struct {
	Install   step.StepList `yaml:"install"   json:"install"`
	Update    step.StepList `yaml:"update"    json:"update"`
	Execute   step.StepList `yaml:"execute"   json:"execute"`
	Stop      step.StepList `yaml:"stop"      json:"stop"`
	Uninstall step.StepList `yaml:"uninstall" json:"uninstall"`
}

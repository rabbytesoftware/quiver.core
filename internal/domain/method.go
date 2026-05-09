package domain

import (
	"github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
)

type Method struct {
	AvailableIn []ArrowState  `yaml:"available_in" json:"available_in"`
	Steps       step.StepList `yaml:"steps"        json:"steps"`
}

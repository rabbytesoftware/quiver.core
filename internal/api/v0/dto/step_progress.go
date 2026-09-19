package dto

import (
	"encoding/json"

	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
)

type StepProgressDTO struct {
	Index  int     `json:"index" yaml:"index"`
	Status string  `json:"status" yaml:"status"`
	Title  string  `json:"title" yaml:"title"`
	Type   string  `json:"type" yaml:"type"`
	Error  *string `json:"error,omitempty" yaml:"error,omitempty"`
}

func StepProgressDTOFrom(sp domainRuntime.StepProgress) StepProgressDTO {
	title, stepType := stepMeta(sp.Step)
	return StepProgressDTO{
		Index:  sp.Index,
		Status: string(sp.Status),
		Title:  title,
		Type:   stepType,
		Error:  sp.Error,
	}
}

func stepMeta(s domainStep.Step) (title, stepType string) {
	if s == nil {
		return "", ""
	}
	stepType = string(s.Type())
	data, err := json.Marshal(s)
	if err != nil {
		return "", stepType
	}
	var wire struct {
		Title string `json:"title" yaml:"title"`
	}
	_ = json.Unmarshal(data, &wire)
	return wire.Title, stepType
}

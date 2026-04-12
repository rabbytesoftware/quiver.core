package dto

import (
	"encoding/json"

	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
)

type StepProgressDTO struct {
	Index  int     `json:"index"`
	Status string  `json:"status"`
	Title  string  `json:"title"`
	Type   string  `json:"type"`
	Error  *string `json:"error,omitempty"`
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
		Title string `json:"title"`
	}
	_ = json.Unmarshal(data, &wire)
	return wire.Title, stepType
}

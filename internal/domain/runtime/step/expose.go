package step

import "encoding/json"

type ExposeStep struct {
	BasicStep
	Kind       string   `json:"kind"`
	Name       string   `json:"name"`
	Path       string   `json:"path"`
	Icon       string   `json:"icon,omitempty"`
	Categories []string `json:"categories,omitempty"`
	MediaIcon  string   `json:"media_icon,omitempty"`
}

func NewExposeStep(
	kind string,
	name string,
	path string,
) ExposeStep {
	return ExposeStep{
		BasicStep: newBasicStep(StepTypeExpose, "Expose "+kind+" "+name, false),
		Kind:      kind,
		Name:      name,
		Path:      path,
	}
}

func (s ExposeStep) Resolve(_ string) Step { return s }

func (s ExposeStep) MarshalJSON() ([]byte, error) {
	type wire struct {
		Kind       StepType `json:"type"`
		Title      string   `json:"title"`
		EntryKind  string   `json:"kind"`
		Name       string   `json:"name"`
		Path       string   `json:"path"`
		Icon       string   `json:"icon,omitempty"`
		Categories []string `json:"categories,omitempty"`
		MediaIcon  string   `json:"media_icon,omitempty"`
	}
	return json.Marshal(wire{
		Kind:       s.Type(),
		Title:      s.Title(),
		EntryKind:  s.Kind,
		Name:       s.Name,
		Path:       s.Path,
		Icon:       s.Icon,
		Categories: s.Categories,
		MediaIcon:  s.MediaIcon,
	})
}

func (s *ExposeStep) UnmarshalJSON(
	data []byte,
) error {
	var wire struct {
		Kind       StepType `json:"type"`
		Title      string   `json:"title"`
		EntryKind  string   `json:"kind"`
		Name       string   `json:"name"`
		Path       string   `json:"path"`
		Icon       string   `json:"icon,omitempty"`
		Categories []string `json:"categories,omitempty"`
		MediaIcon  string   `json:"media_icon,omitempty"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	s.BasicStep = newBasicStep(wire.Kind, wire.Title, false)
	s.Kind = wire.EntryKind
	s.Name = wire.Name
	s.Path = wire.Path
	s.Icon = wire.Icon
	s.Categories = wire.Categories
	s.MediaIcon = wire.MediaIcon
	return nil
}

type UnexposeStep struct {
	BasicStep
}

func NewUnexposeStep() UnexposeStep {
	return UnexposeStep{
		BasicStep: newBasicStep(StepTypeUnexpose, "Remove exposed entries", false),
	}
}

func (s UnexposeStep) Resolve(_ string) Step { return s }

func (s UnexposeStep) MarshalJSON() ([]byte, error) {
	type wire struct {
		Kind  StepType `json:"type"`
		Title string   `json:"title"`
	}
	return json.Marshal(wire{
		Kind:  s.Type(),
		Title: s.Title(),
	})
}

func (s *UnexposeStep) UnmarshalJSON(
	data []byte,
) error {
	var wire struct {
		Kind  StepType `json:"type"`
		Title string   `json:"title"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	s.BasicStep = newBasicStep(wire.Kind, wire.Title, false)
	return nil
}

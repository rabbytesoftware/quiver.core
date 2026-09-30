package step

import "encoding/json"

type PortableStep struct {
	BasicStep
	From    Overrideable[string] `json:"from"`
	To      Overrideable[string] `json:"to"`
	Timeout Overrideable[string] `json:"timeout"`
	Name    string               `json:"name,omitempty"`
}

func (s PortableStep) Resolve(
	os string,
) Step {
	s.From = Overrideable[string]{Default: s.From.Resolve(os)}
	s.To = Overrideable[string]{Default: s.To.Resolve(os)}
	s.Timeout = Overrideable[string]{Default: s.Timeout.Resolve(os)}
	return s
}

func NewPortableStep(
	title string,
	from string,
	to string,
	timeout string,
	exitOnFailure bool,
) PortableStep {
	return PortableStep{
		BasicStep: newBasicStep(StepTypePortable, title, exitOnFailure),
		From:      Overrideable[string]{Default: from},
		To:        Overrideable[string]{Default: to},
		Timeout:   Overrideable[string]{Default: timeout},
	}
}

func (s PortableStep) MarshalJSON() ([]byte, error) {
	type wire struct {
		Kind          StepType             `json:"type"`
		Title         string               `json:"title"`
		ExitOnFailure bool                 `json:"exit_on_failure"`
		From          Overrideable[string] `json:"from"`
		To            Overrideable[string] `json:"to"`
		Timeout       Overrideable[string] `json:"timeout"`
		Name          string               `json:"name,omitempty"`
	}
	return json.Marshal(wire{
		Kind:          s.Type(),
		Title:         s.Title(),
		ExitOnFailure: s.ExitOnFailure(),
		From:          s.From,
		To:            s.To,
		Timeout:       s.Timeout,
		Name:          s.Name,
	})
}

func (s *PortableStep) UnmarshalJSON(
	data []byte,
) error {
	var wire struct {
		Kind          StepType             `json:"type"`
		Title         string               `json:"title"`
		ExitOnFailure bool                 `json:"exit_on_failure"`
		From          Overrideable[string] `json:"from"`
		To            Overrideable[string] `json:"to"`
		Timeout       Overrideable[string] `json:"timeout"`
		Name          string               `json:"name,omitempty"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	s.BasicStep = newBasicStep(wire.Kind, wire.Title, wire.ExitOnFailure)
	s.From = wire.From
	s.To = wire.To
	s.Timeout = wire.Timeout
	s.Name = wire.Name
	return nil
}

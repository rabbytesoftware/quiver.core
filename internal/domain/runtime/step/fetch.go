package step

import "encoding/json"

type FetchStep struct {
	BasicStep
	URL      Overrideable[string] `json:"url"`
	To       Overrideable[string] `json:"to"`
	Checksum Overrideable[string] `json:"checksum"`
	Timeout  Overrideable[string] `json:"timeout"`
}

func (s FetchStep) Resolve(os string) Step {
	s.URL = Overrideable[string]{Default: s.URL.Resolve(os)}
	s.To = Overrideable[string]{Default: s.To.Resolve(os)}
	s.Checksum = Overrideable[string]{Default: s.Checksum.Resolve(os)}
	s.Timeout = Overrideable[string]{Default: s.Timeout.Resolve(os)}
	return s
}

func NewFetchStep(
	title string,
	url string,
	to string,
	checksum string,
	timeout string,
	exitOnFailure bool,
) FetchStep {
	return FetchStep{
		BasicStep: newBasicStep(StepTypeFetch, title, exitOnFailure),
		URL:       Overrideable[string]{Default: url},
		To:        Overrideable[string]{Default: to},
		Checksum:  Overrideable[string]{Default: checksum},
		Timeout:   Overrideable[string]{Default: timeout},
	}
}

func (s FetchStep) MarshalJSON() ([]byte, error) {
	type wire struct {
		Kind          StepType             `json:"type"`
		Title         string               `json:"title"`
		ExitOnFailure bool                 `json:"exit_on_failure"`
		URL           Overrideable[string] `json:"url"`
		To            Overrideable[string] `json:"to"`
		Checksum      Overrideable[string] `json:"checksum"`
		Timeout       Overrideable[string] `json:"timeout"`
	}
	return json.Marshal(wire{
		Kind:          s.Type(),
		Title:         s.Title(),
		ExitOnFailure: s.ExitOnFailure(),
		URL:           s.URL,
		To:            s.To,
		Checksum:      s.Checksum,
		Timeout:       s.Timeout,
	})
}

func (s *FetchStep) UnmarshalJSON(data []byte) error {
	var wire struct {
		Kind          StepType             `json:"type"`
		Title         string               `json:"title"`
		ExitOnFailure bool                 `json:"exit_on_failure"`
		URL           Overrideable[string] `json:"url"`
		To            Overrideable[string] `json:"to"`
		Checksum      Overrideable[string] `json:"checksum"`
		Timeout       Overrideable[string] `json:"timeout"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	s.BasicStep = newBasicStep(wire.Kind, wire.Title, wire.ExitOnFailure)
	s.URL = wire.URL
	s.To = wire.To
	s.Checksum = wire.Checksum
	s.Timeout = wire.Timeout
	return nil
}

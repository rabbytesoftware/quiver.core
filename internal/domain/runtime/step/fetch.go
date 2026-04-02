package step

import (
	"encoding/json"
	"time"
)

type FetchStep struct {
	Kind          StepType             `json:"type"`
	Title         string               `json:"title"`
	exitOnFailure bool                 `json:"exit_on_failure"`
	URL           Overrideable[string] `json:"url"`
	To            Overrideable[string] `json:"to"`
	Timeout       Overrideable[string] `json:"timeout"`
}

// NewFetchStep creates a FetchStep with the given values.
func NewFetchStep(
	title string,
	url string,
	to string,
	timeout time.Duration,
	exitOnFailure bool,
) FetchStep {
	timeoutStr := ""
	if timeout > 0 {
		timeoutStr = timeout.String()
	}
	return FetchStep{
		Kind:          StepTypeFetch,
		Title:         title,
		exitOnFailure: exitOnFailure,
		URL:           Overrideable[string]{Default: url},
		To:            Overrideable[string]{Default: to},
		Timeout:       Overrideable[string]{Default: timeoutStr},
	}
}

func (s FetchStep) Type() StepType      { return s.Kind }
func (s FetchStep) ExitOnFailure() bool { return s.exitOnFailure }

func (s FetchStep) MarshalJSON() ([]byte, error) {
	type wire struct {
		Kind          StepType             `json:"type"`
		Title         string               `json:"title"`
		ExitOnFailure bool                 `json:"exit_on_failure"`
		URL           Overrideable[string] `json:"url"`
		To            Overrideable[string] `json:"to"`
		Timeout       Overrideable[string] `json:"timeout"`
	}
	return json.Marshal(wire{
		Kind:          s.Kind,
		Title:         s.Title,
		ExitOnFailure: s.exitOnFailure,
		URL:           s.URL,
		To:            s.To,
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
		Timeout       Overrideable[string] `json:"timeout"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	s.Kind = wire.Kind
	s.Title = wire.Title
	s.exitOnFailure = wire.ExitOnFailure
	s.URL = wire.URL
	s.To = wire.To
	s.Timeout = wire.Timeout
	return nil
}

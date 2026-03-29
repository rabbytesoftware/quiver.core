package step

import (
	"encoding/json"
	"time"
)

type FetchStep struct {
	BasicStep
	URL     string        `json:"url"`
	To      string        `json:"to"`
	Timeout time.Duration `json:"timeout"`
}

func NewFetchStep(
	title string,
	url string,
	to string,
	timeout time.Duration,
	exitOnFailure bool,
) FetchStep {
	return FetchStep{
		BasicStep: BasicStep{stepType: StepTypeFetch, exitOnFailure: exitOnFailure, title: title},
		URL:       url,
		To:        to,
		Timeout:   timeout,
	}
}

func (s FetchStep) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type          StepType      `json:"type"`
		Title         string        `json:"title"`
		ExitOnFailure bool          `json:"exit_on_failure"`
		URL           string        `json:"url"`
		To            string        `json:"to"`
		Timeout       time.Duration `json:"timeout"`
	}{
		Type:          s.stepType,
		Title:         s.title,
		ExitOnFailure: s.exitOnFailure,
		URL:           s.URL,
		To:            s.To,
		Timeout:       s.Timeout,
	})
}

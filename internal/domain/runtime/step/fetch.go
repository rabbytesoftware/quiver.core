package step

import "time"

type FetchStep struct {
	Kind          StepType             `json:"type"`
	Title         string               `json:"title"`
	ExitOnFailure bool                 `json:"exit_on_failure"`
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
		ExitOnFailure: exitOnFailure,
		URL:           Overrideable[string]{Default: url},
		To:            Overrideable[string]{Default: to},
		Timeout:       Overrideable[string]{Default: timeoutStr},
	}
}

func (s FetchStep) Type() StepType { return s.Kind }

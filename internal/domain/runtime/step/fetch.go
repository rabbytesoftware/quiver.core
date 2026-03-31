package step

import "time"

type FetchStep struct {
	Title         string             `yaml:"title"          json:"title"`
	ExitOnFailure bool               `yaml:"exit_on_failure" json:"exit_on_failure"`
	URL           OverrideableString `yaml:"url"            json:"url"`
	To            OverrideableString `yaml:"to"             json:"to"`
	Timeout       OverrideableString `yaml:"timeout"        json:"timeout"`
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
		Title:         title,
		ExitOnFailure: exitOnFailure,
		URL:           OverrideableString{Default: url},
		To:            OverrideableString{Default: to},
		Timeout:       OverrideableString{Default: timeoutStr},
	}
}

func (s FetchStep) Type() StepType { return StepTypeFetch }

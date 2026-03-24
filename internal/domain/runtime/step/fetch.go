package step

import "time"

type FetchStep struct {
	BasicStep
	URL     string
	To      string
	Timeout time.Duration
}

func NewFetchStep(title, url, to string, timeout time.Duration, exitOnFailure bool) FetchStep {
	return FetchStep{
		BasicStep: BasicStep{stepType: StepTypeFetch, exitOnFailure: exitOnFailure, title: title},
		URL:       url,
		To:        to,
		Timeout:   timeout,
	}
}

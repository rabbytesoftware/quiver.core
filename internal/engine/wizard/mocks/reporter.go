package mocks

// Reporter is a recording mock for wizard.StepReporter.
// Calls are appended to the exported slices; override behavior with the Func fields.
type Reporter struct {
	Started   []int
	Completed []int
	Failed    []int
	FailErrs  []error

	OnStepStartedFunc   func(index int)
	OnStepCompletedFunc func(index int)
	OnStepFailedFunc    func(index int, err error)
}

func (r *Reporter) OnStepStarted(index int) {
	r.Started = append(r.Started, index)
	if r.OnStepStartedFunc != nil {
		r.OnStepStartedFunc(index)
	}
}

func (r *Reporter) OnStepCompleted(index int) {
	r.Completed = append(r.Completed, index)
	if r.OnStepCompletedFunc != nil {
		r.OnStepCompletedFunc(index)
	}
}

func (r *Reporter) OnStepFailed(index int, err error) {
	r.Failed = append(r.Failed, index)
	r.FailErrs = append(r.FailErrs, err)
	if r.OnStepFailedFunc != nil {
		r.OnStepFailedFunc(index, err)
	}
}

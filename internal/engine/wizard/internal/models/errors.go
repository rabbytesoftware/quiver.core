package models

import "errors"

var ErrUnknownStepType = errors.New("wizard: unknown step type")

// ErrShuttingDown is returned by a synchronous run that arrives after the
// wizard has begun shutting down. An asynchronous Start reports the same
// condition as a cancelled outcome instead, because it has an Execution to
// report it on; a synchronous caller has only the error.
var ErrShuttingDown = errors.New("wizard: shutting down")

// ErrVacuousProbe is returned by Probe when it is handed something that could
// only ever answer "yes": no steps at all, or a run step whose command resolves
// to nothing. `sh -c ""` exits 0, so such a step reports a successful detection
// from a command that never ran — and a probe's success means "this software is
// already installed, mark the arrow Ready and skip installing it". A probe must
// never fail open into that, so an unanswerable question is answered "not
// detected", which is the ordinary negative result its caller already handles.
//
// The validator rejects both shapes at parse time (OverrideableCoverageRule for
// the empty command, and a probe is only ever built from a non-empty
// preinstalled block). This is the floor under that, for the same reason
// maxProbeDuration is a hard ceiling under the timeout rules: a future
// validator gap, or a path that bypasses validation, must degrade to "not
// detected" rather than to a silent install.
var ErrVacuousProbe = errors.New("wizard: probe has nothing to verify")

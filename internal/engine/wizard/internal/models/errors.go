package models

import "errors"

var ErrUnknownStepType = errors.New("wizard: unknown step type")

// ErrShuttingDown is returned by a synchronous run that arrives after the
// wizard has begun shutting down. An asynchronous Start reports the same
// condition as a cancelled outcome instead, because it has an Execution to
// report it on; a synchronous caller has only the error.
var ErrShuttingDown = errors.New("wizard: shutting down")

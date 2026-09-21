package client

import (
	"errors"
	"fmt"
)

// Exit codes the CLI process reports, per docs/spec § 6.6.
const (
	ExitOK         = 0
	ExitFailed     = 1
	ExitUsage      = 2
	ExitConnection = 3
)

// APIError is a non-2xx (or success=false) response from the daemon.
type APIError struct {
	Status    int
	Message   string
	Namespace string
}

func (e *APIError) Error() string {
	if e.Namespace != "" {
		return fmt.Sprintf("%s (%s)", e.Message, e.Namespace)
	}
	return e.Message
}

// ConnError means the daemon (or remote instance) could not be reached.
type ConnError struct {
	Server string
	Err    error
}

func (e *ConnError) Error() string {
	return fmt.Sprintf("cannot reach daemon at %s: %v", e.Server, e.Err)
}

func (e *ConnError) Unwrap() error { return e.Err }

// ExitCode maps an error to the CLI process exit code.
func ExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	var connErr *ConnError
	if errors.As(err, &connErr) {
		return ExitConnection
	}
	return ExitFailed
}

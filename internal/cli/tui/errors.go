// Package tui holds the CLI's shared rendering contract: the CommandModel
// every command satisfies, and the Runner that draws and serializes it.
package tui

import (
	"errors"
	"fmt"
)

// Process exit codes.
const (
	// ExitOK reports success.
	ExitOK = 0
	// ExitFailure reports a command that ran and failed.
	ExitFailure = 1
	// ExitUsage reports a malformed invocation.
	ExitUsage = 2
	// ExitUnreachable reports a daemon that could not be contacted.
	ExitUnreachable = 3
)

type usageError struct{ msg string }

func (e usageError) Error() string { return e.msg }

// Usage returns an error that exits with ExitUsage.
func Usage(format string, a ...any) error {
	return usageError{msg: fmt.Sprintf(format, a...)}
}

type connError struct {
	addr string
	err  error
}

func (e connError) Error() string {
	return fmt.Sprintf("cannot reach daemon at %s: %v", e.addr, e.err)
}

func (e connError) Unwrap() error { return e.err }

// Conn returns an error that exits with ExitUnreachable.
func Conn(addr string, err error) error {
	return connError{addr: addr, err: err}
}

// CodeFor maps err to the process exit code it should produce.
func CodeFor(err error) int {
	if err == nil {
		return ExitOK
	}

	var ue usageError
	if errors.As(err, &ue) {
		return ExitUsage
	}

	var ce connError
	if errors.As(err, &ce) {
		return ExitUnreachable
	}

	return ExitFailure
}

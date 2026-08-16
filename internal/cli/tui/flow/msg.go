// Package flow holds the three command lifecycles every CLI command runs on,
// plus the Confirm decorator.
package flow

// readyMsg carries a completed one-shot result.
type readyMsg[T any] struct{ data T }

// errMsg carries a terminal error from any command.
type errMsg struct{ err error }

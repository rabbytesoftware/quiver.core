// Package flow holds the three command lifecycles every CLI command runs on,
// plus the Confirm decorator.
package flow

// readyMsg carries a completed one-shot result.
type readyMsg[T any] struct{ data T }

// errMsg carries a terminal error from any command.
type errMsg struct{ err error }

// openedMsg carries a freshly opened event stream.
type openedMsg[T any] struct{ ch <-chan Event[T] }

// eventMsg carries one event read off that stream.
type eventMsg[T any] struct{ ev Event[T] }

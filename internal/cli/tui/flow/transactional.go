package flow

import (
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
)

// Transactional is the flow for commands that mutate state and report an
// outcome. It is an alias for Instant because their mechanics are identical:
// one round trip, one terminal result. The separate constructor names the
// intent at call sites and lets the two diverge without touching them.
type Transactional[T any] = Instant[T]

// TxOpts configures a Transactional flow.
type TxOpts[T any] struct {
	// Label is shown beside the spinner while the mutation is in flight.
	Label string
	// Do performs the mutation. It takes no context: the caller's closure
	// captures it.
	Do func() (T, error)
	// View renders the outcome.
	View func(T, theme.Theme) string
}

// NewTransactional returns a flow that performs o.Do and reports its outcome.
func NewTransactional[T any](th theme.Theme, o TxOpts[T]) *Transactional[T] {
	return NewInstant(th, o.Label, o.Do, o.View)
}

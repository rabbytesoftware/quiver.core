package mocks

import (
	"context"
	"sync"

	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/discovery"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// Browser answers Browse from a canned Answer per call, in order, and records
// every request it was asked. Calls past the last answer repeat the last one.
type Browser struct {
	mu       sync.Mutex
	Answers  []BrowseAnswer
	Requests []discovery.BrowseRequest
	Calls    int
	// OnBrowse, when set, runs at the top of every Browse call, so a test can
	// hold a pass open or cancel its context mid-flight.
	OnBrowse func(ctx context.Context)
}

// BrowseAnswer is one canned pass: what it emits, the outcome it reports, and
// the error it returns.
type BrowseAnswer struct {
	Emit    []discovery.Result
	Outcome discovery.Outcome
	Err     error
}

func (b *Browser) Browse(
	ctx context.Context,
	req discovery.BrowseRequest,
	emit func(discovery.Result),
) (discovery.Outcome, error) {
	if b.OnBrowse != nil {
		b.OnBrowse(ctx)
	}

	answer := b.next(req)
	for _, result := range answer.Emit {
		emit(result)
	}
	return answer.Outcome, answer.Err
}

func (b *Browser) next(
	req discovery.BrowseRequest,
) BrowseAnswer {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.Requests = append(b.Requests, req)
	b.Calls++
	if len(b.Answers) == 0 {
		return BrowseAnswer{}
	}
	return b.Answers[min(b.Calls, len(b.Answers))-1]
}

// Result builds a verified discovery result for ns.
func Result(
	ns string,
	stars int,
	source string,
) discovery.Result {
	return discovery.Result{Namespace: domain.Namespace(ns), Stars: stars, Source: source}
}

// Outcome builds an outcome whose single provider answered, with the given
// search order.
func Outcome(
	order ...string,
) discovery.Outcome {
	namespaces := make([]domain.Namespace, 0, len(order))
	for _, ns := range order {
		namespaces = append(namespaces, domain.Namespace(ns))
	}
	return discovery.Outcome{
		Found:     len(order),
		Providers: []discovery.ProviderOutcome{{Host: "github.com", OK: true, Returned: len(order)}},
		Order:     namespaces,
	}
}

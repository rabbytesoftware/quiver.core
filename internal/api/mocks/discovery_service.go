package mocks

import (
	"context"
	"sync"

	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
)

// DiscoveryService is a hand-written stand-in for usecases.DiscoveryUsecase.
// Its counters are guarded because the routes cancel a pass from the goroutine
// serving the WebSocket, not from the request that started it.
type DiscoveryService struct {
	StartResult usecases.Job
	StartErr    error
	GetResult   *usecases.Job
	GetErr      error

	mu         sync.Mutex
	startCalls int
	startQuery string
	getID      string
	cancelled  []string
	attached   []string
	detached   []string
	replay     []usecases.StreamItem
	done       chan struct{}
	listeners  []func(usecases.StreamItem)
}

func (m *DiscoveryService) Start(
	_ context.Context,
	text string,
) (usecases.Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startCalls++
	m.startQuery = text
	return m.StartResult, m.StartErr
}

func (m *DiscoveryService) Get(
	_ context.Context,
	id string,
) (*usecases.Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getID = id
	return m.GetResult, m.GetErr
}

func (m *DiscoveryService) Cancel(
	_ context.Context,
	id string,
) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cancelled = append(m.cancelled, id)
}

func (m *DiscoveryService) Attach(
	id string,
) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.attached = append(m.attached, id)
}

func (m *DiscoveryService) Detach(
	id string,
) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.detached = append(m.detached, id)
}

func (m *DiscoveryService) Replay(
	_ string,
) []usecases.StreamItem {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]usecases.StreamItem(nil), m.replay...)
}

// Done returns a channel Finish closes, so a test decides when the stream ends.
func (m *DiscoveryService) Done(
	_ string,
) <-chan struct{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.done == nil {
		m.done = make(chan struct{})
	}
	return m.done
}

// Finish marks the job done.
func (m *DiscoveryService) Finish() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.done == nil {
		m.done = make(chan struct{})
	}
	close(m.done)
}

// SetReplay fixes what Replay returns.
func (m *DiscoveryService) SetReplay(
	items []usecases.StreamItem,
) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.replay = items
}

func (m *DiscoveryService) Attached() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.attached...)
}

func (m *DiscoveryService) Detached() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.detached...)
}

func (m *DiscoveryService) OnResult(
	emit func(usecases.StreamItem),
) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listeners = append(m.listeners, emit)
}

// Emit pushes an item to every registered listener, so a test can drive the
// stream without running a real pass.
func (m *DiscoveryService) Emit(
	item usecases.StreamItem,
) {
	m.mu.Lock()
	listeners := make([]func(usecases.StreamItem), len(m.listeners))
	copy(listeners, m.listeners)
	m.mu.Unlock()

	for _, emit := range listeners {
		emit(item)
	}
}

func (m *DiscoveryService) StartCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.startCalls
}

func (m *DiscoveryService) StartQuery() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.startQuery
}

func (m *DiscoveryService) GetID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.getID
}

func (m *DiscoveryService) Cancelled() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.cancelled...)
}

func (m *DiscoveryService) Listeners() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.listeners)
}

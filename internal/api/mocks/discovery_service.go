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

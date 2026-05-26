package mocks

import (
	"sync"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

type Hub struct {
	mu                  sync.Mutex
	BroadcastedArrows   []domain.Arrow
	BroadcastedRuntimes []domainRuntime.ArrowRuntime
	BroadcastedQuivers  []domain.Collection
}

func (m *Hub) BroadcastArrow(arrow domain.Arrow) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.BroadcastedArrows = append(m.BroadcastedArrows, arrow)
}

func (m *Hub) BroadcastArrowRuntime(runtime domainRuntime.ArrowRuntime) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.BroadcastedRuntimes = append(m.BroadcastedRuntimes, runtime)
}

func (m *Hub) BroadcastCollection(quiver domain.Collection) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.BroadcastedQuivers = append(m.BroadcastedQuivers, quiver)
}

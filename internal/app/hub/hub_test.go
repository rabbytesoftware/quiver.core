package hub_test

import (
	"sync"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/app/hub"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
)

type mockSubscriber struct {
	mu            sync.Mutex
	arrows        []domain.Arrow
	arrowRuntimes []domainRuntime.ArrowRuntime
	quivers       []domain.Collection
}

func (m *mockSubscriber) PushArrow(a domain.Arrow) {
	m.mu.Lock()
	m.arrows = append(m.arrows, a)
	m.mu.Unlock()
}

func (m *mockSubscriber) PushArrowRuntime(rt domainRuntime.ArrowRuntime) {
	m.mu.Lock()
	m.arrowRuntimes = append(m.arrowRuntimes, rt)
	m.mu.Unlock()
}

func (m *mockSubscriber) PushCollection(q domain.Collection) {
	m.mu.Lock()
	m.quivers = append(m.quivers, q)
	m.mu.Unlock()
}

func TestNewHub_NotNil(t *testing.T) {
	h := hub.NewHub()
	if h == nil {
		t.Fatal("expected non-nil hub")
	}
}

func TestHub_BroadcastArrow_NoSubscribers(t *testing.T) {
	h := hub.NewHub()
	// Should not panic with no subscribers.
	h.BroadcastArrow(domain.Arrow{Namespace: "test/arrow@v1"})
}

func TestHub_Register_And_BroadcastArrow(t *testing.T) {
	h := hub.NewHub()
	s := &mockSubscriber{}
	h.Register(s)

	arrow := domain.Arrow{Namespace: "test/arrow@v1"}
	h.BroadcastArrow(arrow)

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.arrows) != 1 {
		t.Fatalf("expected 1 arrow broadcast, got %d", len(s.arrows))
	}
	if s.arrows[0].Namespace != arrow.Namespace {
		t.Errorf("unexpected arrow: %v", s.arrows[0])
	}
}

func TestHub_BroadcastArrowRuntime_NoSubscribers(t *testing.T) {
	h := hub.NewHub()
	h.BroadcastArrowRuntime(domainRuntime.ArrowRuntime{Ref: "test/arrow@v1"})
}

func TestHub_Register_And_BroadcastArrowRuntime(t *testing.T) {
	h := hub.NewHub()
	s := &mockSubscriber{}
	h.Register(s)

	rt := domainRuntime.ArrowRuntime{Ref: "test/arrow@v1"}
	h.BroadcastArrowRuntime(rt)

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.arrowRuntimes) != 1 {
		t.Fatalf("expected 1 runtime broadcast, got %d", len(s.arrowRuntimes))
	}
	if s.arrowRuntimes[0].Ref != rt.Ref {
		t.Errorf("unexpected runtime: %v", s.arrowRuntimes[0])
	}
}

func TestHub_BroadcastCollection_NoSubscribers(t *testing.T) {
	h := hub.NewHub()
	h.BroadcastCollection(domain.Collection{})
}

func TestHub_Register_And_BroadcastCollection(t *testing.T) {
	h := hub.NewHub()
	s := &mockSubscriber{}
	h.Register(s)

	q := domain.Collection{}
	h.BroadcastCollection(q)

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.quivers) != 1 {
		t.Fatalf("expected 1 quiver broadcast, got %d", len(s.quivers))
	}
}

func TestHub_MultipleSubscribers(t *testing.T) {
	h := hub.NewHub()
	s1 := &mockSubscriber{}
	s2 := &mockSubscriber{}
	h.Register(s1)
	h.Register(s2)

	arrow := domain.Arrow{Namespace: "test/multi@v1"}
	h.BroadcastArrow(arrow)

	for _, s := range []*mockSubscriber{s1, s2} {
		s.mu.Lock()
		if len(s.arrows) != 1 {
			s.mu.Unlock()
			t.Fatalf("expected 1 arrow, got %d", len(s.arrows))
		}
		s.mu.Unlock()
	}
}

func TestHub_ImplementsWebSocketHub(t *testing.T) {
	h := hub.NewHub()
	var _ hub.WebSocketHub = h
}

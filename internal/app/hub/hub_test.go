package hub_test

import (
	"sync"
	"testing"

	"github.com/rabbytesoftware/quiver.core/internal/app/hub"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

type mockSubscriber struct {
	mu          sync.Mutex
	arrows      []hub.ArrowEvent
	runtimes    []domainRuntime.ArrowRuntime
	collections []hub.CollectionEvent
}

func (m *mockSubscriber) PushArrow(e hub.ArrowEvent) {
	m.mu.Lock()
	m.arrows = append(m.arrows, e)
	m.mu.Unlock()
}

func (m *mockSubscriber) PushArrowRuntime(rt domainRuntime.ArrowRuntime) {
	m.mu.Lock()
	m.runtimes = append(m.runtimes, rt)
	m.mu.Unlock()
}

func (m *mockSubscriber) PushCollection(e hub.CollectionEvent) {
	m.mu.Lock()
	m.collections = append(m.collections, e)
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
	h.BroadcastArrow(hub.ArrowEvent{Kind: hub.CatalogUpserted, Arrow: domain.Arrow{Namespace: "test/arrow@v1"}})
}

func TestHub_BroadcastArrow_Upserted_DeliveredWithKind(t *testing.T) {
	h := hub.NewHub()
	s := &mockSubscriber{}
	h.Register(s)

	evt := hub.ArrowEvent{Kind: hub.CatalogUpserted, Arrow: domain.Arrow{Namespace: "test/arrow@v1"}}
	h.BroadcastArrow(evt)

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.arrows) != 1 {
		t.Fatalf("expected 1 arrow event, got %d", len(s.arrows))
	}
	if s.arrows[0].Kind != hub.CatalogUpserted {
		t.Errorf("expected CatalogUpserted, got %v", s.arrows[0].Kind)
	}
	if s.arrows[0].Namespace != "test/arrow@v1" {
		t.Errorf("unexpected namespace: %v", s.arrows[0].Namespace)
	}
}

func TestHub_BroadcastArrow_Removed_DeliveredWithKind(t *testing.T) {
	h := hub.NewHub()
	s := &mockSubscriber{}
	h.Register(s)

	evt := hub.ArrowEvent{Kind: hub.CatalogRemoved, Arrow: domain.Arrow{Namespace: "test/arrow@v1"}}
	h.BroadcastArrow(evt)

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.arrows) != 1 {
		t.Fatalf("expected 1 arrow event, got %d", len(s.arrows))
	}
	if s.arrows[0].Kind != hub.CatalogRemoved {
		t.Errorf("expected CatalogRemoved, got %v", s.arrows[0].Kind)
	}
}

func TestHub_BroadcastArrowRuntime_NoSubscribers(t *testing.T) {
	h := hub.NewHub()
	h.BroadcastArrowRuntime(domainRuntime.ArrowRuntime{Ref: "test/arrow@v1"})
}

func TestHub_BroadcastArrowRuntime_DeliveredToSubscriber(t *testing.T) {
	h := hub.NewHub()
	s := &mockSubscriber{}
	h.Register(s)

	rt := domainRuntime.ArrowRuntime{Ref: "test/arrow@v1"}
	h.BroadcastArrowRuntime(rt)

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.runtimes) != 1 {
		t.Fatalf("expected 1 runtime broadcast, got %d", len(s.runtimes))
	}
	if s.runtimes[0].Ref != rt.Ref {
		t.Errorf("unexpected runtime: %v", s.runtimes[0])
	}
}

func TestHub_BroadcastCollection_NoSubscribers(t *testing.T) {
	h := hub.NewHub()
	h.BroadcastCollection(hub.CollectionEvent{Kind: hub.CatalogUpserted})
}

func TestHub_BroadcastCollection_Upserted_DeliveredWithKind(t *testing.T) {
	h := hub.NewHub()
	s := &mockSubscriber{}
	h.Register(s)

	evt := hub.CollectionEvent{Kind: hub.CatalogUpserted, Collection: domain.Collection{Namespace: "test/quiver"}}
	h.BroadcastCollection(evt)

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.collections) != 1 {
		t.Fatalf("expected 1 collection event, got %d", len(s.collections))
	}
	if s.collections[0].Kind != hub.CatalogUpserted {
		t.Errorf("expected CatalogUpserted, got %v", s.collections[0].Kind)
	}
	if s.collections[0].Namespace != "test/quiver" {
		t.Errorf("unexpected namespace: %v", s.collections[0].Namespace)
	}
}

func TestHub_BroadcastCollection_Removed_DeliveredWithKind(t *testing.T) {
	h := hub.NewHub()
	s := &mockSubscriber{}
	h.Register(s)

	evt := hub.CollectionEvent{Kind: hub.CatalogRemoved, Collection: domain.Collection{Namespace: "test/quiver"}}
	h.BroadcastCollection(evt)

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.collections) != 1 {
		t.Fatalf("expected 1 collection event, got %d", len(s.collections))
	}
	if s.collections[0].Kind != hub.CatalogRemoved {
		t.Errorf("expected CatalogRemoved, got %v", s.collections[0].Kind)
	}
}

func TestHub_MultipleSubscribers_AllReceiveArrowEvent(t *testing.T) {
	h := hub.NewHub()
	s1 := &mockSubscriber{}
	s2 := &mockSubscriber{}
	h.Register(s1)
	h.Register(s2)

	evt := hub.ArrowEvent{Kind: hub.CatalogUpserted, Arrow: domain.Arrow{Namespace: "test/multi@v1"}}
	h.BroadcastArrow(evt)

	for _, s := range []*mockSubscriber{s1, s2} {
		s.mu.Lock()
		if len(s.arrows) != 1 {
			s.mu.Unlock()
			t.Fatalf("expected 1 arrow event, got %d", len(s.arrows))
		}
		s.mu.Unlock()
	}
}

func TestHub_ImplementsWebSocketHub(t *testing.T) {
	h := hub.NewHub()
	var _ hub.WebSocketHub = h
}

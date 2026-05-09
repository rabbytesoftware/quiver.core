package mocks

import (
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

type Hub struct {
	BroadcastArrowFn        func(arrow domain.Arrow)
	BroadcastArrowRuntimeFn func(runtime domainRuntime.ArrowRuntime)
	BroadcastCollectionFn   func(coll domain.Collection)
}

func (m *Hub) BroadcastArrow(arrow domain.Arrow) {
	if m.BroadcastArrowFn != nil {
		m.BroadcastArrowFn(arrow)
	}
}

func (m *Hub) BroadcastArrowRuntime(runtime domainRuntime.ArrowRuntime) {
	if m.BroadcastArrowRuntimeFn != nil {
		m.BroadcastArrowRuntimeFn(runtime)
	}
}

func (m *Hub) BroadcastCollection(coll domain.Collection) {
	if m.BroadcastCollectionFn != nil {
		m.BroadcastCollectionFn(coll)
	}
}

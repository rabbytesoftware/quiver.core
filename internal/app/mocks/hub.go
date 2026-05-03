package mocks

import (
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
)

type Hub struct {
	BroadcastArrowFn        func(arrow domain.Arrow)
	BroadcastArrowRuntimeFn func(runtime domainRuntime.ArrowRuntime)
	BroadcastQuiverFn       func(quiver domain.Quiver)
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

func (m *Hub) BroadcastQuiver(quiver domain.Quiver) {
	if m.BroadcastQuiverFn != nil {
		m.BroadcastQuiverFn(quiver)
	}
}

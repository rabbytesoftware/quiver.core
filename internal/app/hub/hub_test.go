package hub_test

import (
	"testing"

	"github.com/rabbytesoftware/quiver/internal/app/hub"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/stretchr/testify/assert"
)

// WebSocketHub is a compile-time interface check — any type satisfying it compiles.
func TestWebSocketHub_Interface(t *testing.T) {
	var _ hub.WebSocketHub = (*stubHub)(nil)
	assert.True(t, true) // interface satisfied by stub
}

type stubHub struct{}

func (s *stubHub) BroadcastArrow(_ domain.Arrow)                        {}
func (s *stubHub) BroadcastArrowRuntime(_ domainRuntime.ArrowRuntime)   {}
func (s *stubHub) BroadcastQuiver(_ domain.Quiver)                      {}

package api_test

import (
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rabbytesoftware/quiver/internal/api"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

type stubVersion struct {
	arrows   []domain.Arrow
	runtimes []domainRuntime.ArrowRuntime
	quivers  []domain.Quiver
}

func (s *stubVersion) PushArrow(a domain.Arrow) { s.arrows = append(s.arrows, a) }
func (s *stubVersion) PushArrowRuntime(r domainRuntime.ArrowRuntime) {
	s.runtimes = append(s.runtimes, r)
}
func (s *stubVersion) PushQuiver(q domain.Quiver) { s.quivers = append(s.quivers, q) }

func TestHub_BroadcastArrow_FansOutToAllVersions(t *testing.T) {
	stub1 := &stubVersion{}
	stub2 := &stubVersion{}
	hub := api.NewHub()
	hub.Register(stub1)
	hub.Register(stub2)

	hub.BroadcastArrow(domain.Arrow{Namespace: "github.com/user/repo"})

	assert.Len(t, stub1.arrows, 1)
	assert.Len(t, stub2.arrows, 1)
}

func TestHub_BroadcastArrowRuntime(t *testing.T) {
	stub := &stubVersion{}
	hub := api.NewHub()
	hub.Register(stub)

	hub.BroadcastArrowRuntime(domainRuntime.ArrowRuntime{
		Ref:   "github.com/user/repo",
		State: domain.ArrowStateRunning,
	})

	assert.Len(t, stub.runtimes, 1)
	assert.Equal(t, domain.ArrowStateRunning, stub.runtimes[0].State)
}

func TestHub_BroadcastQuiver(t *testing.T) {
	stub := &stubVersion{}
	hub := api.NewHub()
	hub.Register(stub)

	hub.BroadcastQuiver(domain.Quiver{Namespace: "github.com/user/repo"})

	assert.Len(t, stub.quivers, 1)
}

func TestHub_NoPanic(t *testing.T) {
	hub := api.NewHub()
	hub.BroadcastArrow(domain.Arrow{})
	hub.BroadcastArrowRuntime(domainRuntime.ArrowRuntime{})
	hub.BroadcastQuiver(domain.Quiver{})
}

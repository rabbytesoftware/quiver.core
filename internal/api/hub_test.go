package api_test

import (
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/api"
	apphub "github.com/rabbytesoftware/quiver.core/internal/app/hub"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

type stubVersion struct {
	arrows   []apphub.ArrowEvent
	runtimes []domainRuntime.ArrowRuntime
	quivers  []apphub.CollectionEvent
}

func (s *stubVersion) PushArrow(e apphub.ArrowEvent) { s.arrows = append(s.arrows, e) }
func (s *stubVersion) PushArrowRuntime(r domainRuntime.ArrowRuntime) {
	s.runtimes = append(s.runtimes, r)
}
func (s *stubVersion) PushCollection(e apphub.CollectionEvent) { s.quivers = append(s.quivers, e) }

func TestHub_BroadcastArrow_FansOutToAllVersions(t *testing.T) {
	stub1 := &stubVersion{}
	stub2 := &stubVersion{}
	hub := api.NewHub()
	hub.Register(stub1)
	hub.Register(stub2)

	hub.BroadcastArrow(apphub.ArrowEvent{Kind: apphub.CatalogUpserted, Arrow: domain.Arrow{Namespace: "github.com/user/repo"}})

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

func TestHub_BroadcastCollection(t *testing.T) {
	stub := &stubVersion{}
	hub := api.NewHub()
	hub.Register(stub)

	hub.BroadcastCollection(apphub.CollectionEvent{Kind: apphub.CatalogUpserted, Collection: domain.Collection{Namespace: "github.com/user/repo"}})

	assert.Len(t, stub.quivers, 1)
}

func TestHub_NoPanic(t *testing.T) {
	hub := api.NewHub()
	hub.BroadcastArrow(apphub.ArrowEvent{})
	hub.BroadcastArrowRuntime(domainRuntime.ArrowRuntime{})
	hub.BroadcastCollection(apphub.CollectionEvent{})
}

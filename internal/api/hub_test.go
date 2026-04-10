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

func (s *stubVersion) PushArrow(a domain.Arrow)                      { s.arrows = append(s.arrows, a) }
func (s *stubVersion) PushArrowRuntime(r domainRuntime.ArrowRuntime) { s.runtimes = append(s.runtimes, r) }
func (s *stubVersion) PushQuiver(q domain.Quiver)                    { s.quivers = append(s.quivers, q) }

func TestHub_BroadcastArrow_FansOutToAllVersions(t *testing.T) {
	v1 := &stubVersion{}
	v2 := &stubVersion{}
	hub := api.NewHub(v1, v2)

	arrow := domain.Arrow{Namespace: "github.com/user/repo"}
	hub.BroadcastArrow(arrow)

	assert.Len(t, v1.arrows, 1)
	assert.Len(t, v2.arrows, 1)
}

func TestHub_BroadcastArrowRuntime(t *testing.T) {
	v1 := &stubVersion{}
	hub := api.NewHub(v1)

	rt := domainRuntime.ArrowRuntime{
		Namespace: "github.com/user/repo",
		State:     domain.ArrowStateRunning,
	}
	hub.BroadcastArrowRuntime(rt)

	assert.Len(t, v1.runtimes, 1)
	assert.Equal(t, domain.ArrowStateRunning, v1.runtimes[0].State)
}

func TestHub_BroadcastQuiver(t *testing.T) {
	v1 := &stubVersion{}
	hub := api.NewHub(v1)

	q := domain.Quiver{Namespace: "github.com/user/repo"}
	hub.BroadcastQuiver(q)

	assert.Len(t, v1.quivers, 1)
}

func TestHub_NoVersions(t *testing.T) {
	hub := api.NewHub()
	// Should not panic with no versions registered
	hub.BroadcastArrow(domain.Arrow{})
	hub.BroadcastArrowRuntime(domainRuntime.ArrowRuntime{})
	hub.BroadcastQuiver(domain.Quiver{})
}

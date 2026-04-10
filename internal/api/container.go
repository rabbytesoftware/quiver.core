package api

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rabbytesoftware/quiver/internal/api/middleware"
	apiv0 "github.com/rabbytesoftware/quiver/internal/api/v0"
	"github.com/rabbytesoftware/quiver/internal/app"
)

// Container holds the Gin engine. Obtain via Init — do not construct directly.
type Container struct {
	engine *gin.Engine
}

// Init builds the HTTP layer from an already-wired app container and hub.
// The hub already holds each version's WS handler (registered in NewHub).
// Adding a new API version means adding its routes here — internal.go never changes.
func Init(
	appContainer *app.Container,
	wshub *Hub,
) (*Container, error) {
	if appContainer == nil {
		return nil, fmt.Errorf("api: app container is required")
	}

	v0Container, err := apiv0.Init(appContainer, wshub.v0)
	if err != nil {
		return nil, fmt.Errorf("api: v0: %w", err)
	}

	r := gin.New()
	r.UseRawPath = true
	r.UnescapePathValues = true

	r.Use(middleware.RequestLogger())
	r.Use(middleware.RequestTimer())
	r.Use(middleware.RequestRecovery())

	v0Container.Register(r.Group("/v0"))

	return &Container{engine: r}, nil
}

func (c *Container) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.engine.ServeHTTP(w, r)
}

func (c *Container) Run(addr string) error {
	return c.engine.Run(addr)
}

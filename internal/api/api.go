package api

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rabbytesoftware/quiver/internal/api/middleware"
	apiv0 "github.com/rabbytesoftware/quiver/internal/api/v0"
	"github.com/rabbytesoftware/quiver/internal/app"
)

type Container struct {
	engine *gin.Engine
}

func NewContainer() *Container {
	r := gin.New()

	// REQUIRED: namespace path params contain '/' encoded as %2F.
	// Without these flags gin splits the URL before decoding, causing 404s.
	r.UseRawPath = true
	r.UnescapePathValues = true
	r.Use(middleware.RequestLogger())
	r.Use(middleware.RequestTimer())
	r.Use(middleware.RequestRecovery())
	return &Container{engine: r}
}

func (c *Container) Engine() *gin.Engine {
	return c.engine
}

func (c *Container) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.engine.ServeHTTP(w, r)
}

func (c *Container) Run(addr string) error {
	return c.engine.Run(addr)
}

func Init(
	appContainer *app.Container,
	v1Container *apiv0.Container,
) (*Container, error) {
	if appContainer == nil {
		return nil, fmt.Errorf("api: app container is required")
	}

	c := NewContainer()

	v1Group := c.engine.Group("/v0")
	v1Container.Register(v1Group)

	return c, nil
}

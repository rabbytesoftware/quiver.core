package api

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	apiv1 "github.com/rabbytesoftware/quiver/internal/api/v1"
	"github.com/rabbytesoftware/quiver/internal/api/middleware"
	"github.com/rabbytesoftware/quiver/internal/app"
)

// Container holds the Gin engine.
type Container struct {
	engine *gin.Engine
}

// NewContainer creates the gin engine with required routing flags.
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

// Engine returns the underlying gin.Engine for route registration.
func (c *Container) Engine() *gin.Engine {
	return c.engine
}

// ServeHTTP implements http.Handler.
func (c *Container) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.engine.ServeHTTP(w, r)
}

// Run starts the HTTP server on the given address.
func (c *Container) Run(addr string) error {
	return c.engine.Run(addr)
}

// Init builds the API container from an app container and v1 container.
func Init(
	appContainer *app.Container,
	v1Container *apiv1.Container,
) (*Container, error) {
	if appContainer == nil {
		return nil, fmt.Errorf("api: app container is required")
	}
	c := NewContainer()
	v1Group := c.engine.Group("/v1")
	v1Container.Register(v1Group)
	return c, nil
}

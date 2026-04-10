package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rabbytesoftware/quiver/internal/api/middleware"
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

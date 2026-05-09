package api

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/rabbytesoftware/quiver.core/internal/api/middleware"
	"github.com/rabbytesoftware/quiver.core/internal/core/config"
)

// Container holds the Gin engine. Obtain via New — do not construct directly.
type Container struct {
	engine *gin.Engine
}

// New builds the HTTP layer from an already-wired hub and one or more API versions.
// Adding a new API version means passing it here — this file never changes again.
func New(
	wshub *Hub,
	versions ...Version,
) (*Container, error) {
	if len(versions) == 0 {
		return nil, fmt.Errorf("api: at least one version is required")
	}

	r := gin.New()
	r.UseRawPath = true
	r.UnescapePathValues = true
	r.Use(middleware.RequestLogger())
	r.Use(middleware.RequestTimer())
	r.Use(middleware.RequestRecovery())

	for _, v := range versions {
		wshub.Register(v.WSHandler())
		v.Register(r.Group(v.Prefix()))
	}

	return &Container{engine: r}, nil
}

func (c *Container) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.engine.ServeHTTP(w, r)
}

func (c *Container) Run(
	host string,
	port int,
) error {
	addr := buildAddr(host, port, config.GetAPI())
	return c.engine.Run(addr)
}

// buildAddr resolves the final bind address from CLI overrides and config defaults.
// An empty host or zero port means "use the config value".
func buildAddr(
	host string,
	port int,
	cfg config.API,
) string {
	if host == "" {
		host = cfg.Host
	}
	if port == 0 {
		port = cfg.Port
	}
	return fmt.Sprintf("%s:%d", host, port)
}

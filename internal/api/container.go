package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/rabbytesoftware/quiver.core/internal/api/endpoints/ping"
	"github.com/rabbytesoftware/quiver.core/internal/api/endpoints/versions"
	"github.com/rabbytesoftware/quiver.core/internal/api/middleware"
)

// Container holds the HTTP server. Obtain via New — do not construct directly.
type Container struct {
	server *http.Server
}

// New builds the HTTP layer from an already-wired hub, build info, and one or more API versions.
// Adding a new API version means passing it here — this file never changes again.
func New(
	wshub *Hub,
	buildInfo BuildInfo,
	vs ...Version,
) (*Container, error) {
	if len(vs) == 0 {
		return nil, fmt.Errorf("api: at least one version is required")
	}

	r := gin.New()
	r.UseRawPath = true
	r.UnescapePathValues = true
	r.Use(middleware.RequestLogger())
	r.Use(middleware.RequestTimer())
	r.Use(middleware.RequestRecovery())

	supported := make([]string, len(vs))
	for i, v := range vs {
		supported[i] = strings.TrimPrefix(v.Prefix(), "/")
	}
	latest := supported[len(supported)-1]

	vh := versions.New(buildInfo.Version, buildInfo.BuildID, supported, latest)
	ph := ping.New(buildInfo)
	r.GET("/ping", ph.Get)
	r.GET("/versions", vh.Get)

	for _, v := range vs {
		wshub.Register(v.WSHandler())
		v.Register(r.Group(v.Prefix()))
	}

	return &Container{server: &http.Server{Handler: r, ReadHeaderTimeout: 30 * time.Second}}, nil
}

// ServeHTTP implements http.Handler.
func (c *Container) ServeHTTP(
	w http.ResponseWriter,
	r *http.Request,
) {
	c.server.Handler.ServeHTTP(w, r)
}

// Run starts serving on the provided listener and blocks until the server stops.
func (c *Container) Run(
	listener net.Listener,
) error {
	return c.server.Serve(listener)
}

// Shutdown gracefully drains in-flight requests and stops the server.
func (c *Container) Shutdown(
	ctx context.Context,
) error {
	return c.server.Shutdown(ctx)
}

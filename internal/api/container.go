package api

import (
	"fmt"
	"net/http"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/gin-gonic/gin"
	"github.com/rabbytesoftware/quiver/internal/api/middleware"
	apiv0 "github.com/rabbytesoftware/quiver/internal/api/v0"
	"github.com/rabbytesoftware/quiver/internal/app"
	"github.com/rabbytesoftware/quiver/internal/engine"
)

// Container holds the Gin engine. Obtain via Init — do not construct directly.
type Container struct {
	engine *gin.Engine
}

// Init wires the full API layer. Internally creates the WebSocket hub, the
// app container, and the v0 container. Callers need no knowledge of any of those.
func Init(
	engines *engine.Container,
	arrowES asynxModels.Store,
	runtimeES asynxModels.Store,
	quiverES asynxModels.Store,
) (*Container, error) {
	wsHandler := apiv0.NewWSHandler()
	hub := NewHub(wsHandler)

	appContainer, err := app.Init(engines, arrowES, runtimeES, quiverES, hub)
	if err != nil {
		return nil, fmt.Errorf("api: app: %w", err)
	}

	v0Container, err := apiv0.Init(appContainer, wsHandler)
	if err != nil {
		return nil, fmt.Errorf("api: v0: %w", err)
	}

	r := gin.New()
	r.UseRawPath = true
	r.UnescapePathValues = true
	r.Use(middleware.RequestLogger())
	r.Use(middleware.RequestTimer())
	r.Use(middleware.RequestRecovery())

	v0Group := r.Group("/v0")
	v0Container.Register(v0Group)

	return &Container{engine: r}, nil
}

// ServeHTTP implements http.Handler.
func (c *Container) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.engine.ServeHTTP(w, r)
}

// Run starts the HTTP server on the given address (e.g. ":8080").
func (c *Container) Run(addr string) error {
	return c.engine.Run(addr)
}

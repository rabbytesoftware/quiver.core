package api

import (
	"fmt"
	"io"
	"sync"

	"github.com/rabbytesoftware/quiver/internal/api/middleware"
	"github.com/rabbytesoftware/quiver/internal/core/config"
	"github.com/rabbytesoftware/quiver/internal/core/watcher"

	"github.com/gin-gonic/gin"
	v1 "github.com/rabbytesoftware/quiver/internal/api/v1"
	"github.com/rabbytesoftware/quiver/internal/usecases"
)

type API struct {
	router   *gin.Engine
	usecases *usecases.Usecases
}

var once sync.Once

func NewAPI(
	usecases *usecases.Usecases,
) *API {
	once.Do(func() {
		gin.DefaultWriter = io.Discard
		gin.DefaultErrorWriter = io.Discard
	})

	watcherConfig := watcher.GetConfig()
	if !watcherConfig.Enabled {
		gin.SetMode(gin.ReleaseMode)
	} else {
		level := watcher.GetLevel()
		if level <= 4 {
			gin.SetMode(gin.DebugMode)
		} else {
			gin.SetMode(gin.ReleaseMode)
		}
	}

	return &API{
		router:   gin.New(),
		usecases: usecases,
	}
}

func (a *API) Run() {
	a.SetupMiddleware()
	a.SetupRoutes()

	watcher.Info(fmt.Sprintf(
		"Initializing API on %s:%d",
		config.GetAPI().Host,
		config.GetAPI().Port,
	))

	if err := a.router.Run(
		fmt.Sprintf("%s:%d", config.GetAPI().Host, config.GetAPI().Port),
	); err != nil {
		watcher.Unforeseen("Failed to start API server: %v", err.Error())
	}
}

func (a *API) SetupMiddleware() {
	a.router.Use(middleware.WatcherLogger())
	a.router.Use(middleware.WatcherRecovery())
}

func (a *API) SetupRoutes() {
	v1.SetupRoutes(a.router, a.usecases)
}

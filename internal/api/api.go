package api

import (
	"fmt"
	"io"
	"sync"

	"github.com/rabbytesoftware/quiver/internal/api/middleware"
	"github.com/rabbytesoftware/quiver/internal/api/v1/usecases"
	"github.com/rabbytesoftware/quiver/internal/core/config"
	"github.com/rabbytesoftware/quiver/internal/core/watcher"

	"github.com/gin-gonic/gin"
	v1 "github.com/rabbytesoftware/quiver/internal/api/v1"

	swaggerfiles "github.com/swaggo/files"     // swagger embed files
	ginSwagger "github.com/swaggo/gin-swagger" // gin-swagger middleware
	docs "github.com/rabbytesoftware/quiver/docs" // swagger docs
)

type API struct {
	router   *gin.Engine
	usecases *usecases.ApiUsescases
}

var once sync.Once

func NewAPI(
	usecases *usecases.ApiUsescases,
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

	// Setup swagger docs
	docs.SwaggerInfo.BasePath = "/api"
	a.router.GET("/docs/*any", ginSwagger.WrapHandler(swaggerfiles.Handler))
	
	apiUrl := fmt.Sprintf("%s:%d", config.GetAPI().Host, config.GetAPI().Port)

	watcher.Info(fmt.Sprintf(
		"\nInitializing API on %s\nDocs path: %s",
		apiUrl,
		fmt.Sprintf("%s/docs/index.html", apiUrl),
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

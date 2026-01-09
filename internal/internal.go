package internal

import (
	"github.com/rabbytesoftware/quiver/internal/api"
	"github.com/rabbytesoftware/quiver/internal/usecases"
	"github.com/rabbytesoftware/quiver/internal/core"
	"github.com/rabbytesoftware/quiver/internal/infrastructure"
	"github.com/rabbytesoftware/quiver/internal/repositories"
)

// ? Internal DI Container
// ? This is the initiator for internal services (Dependency Injection)

// ? This case is spectial, because here we only expose the core services to the outside world.
// ? All other services are internal and are not exposed to the outside world as they are
// ? essential for the internal workings of the application and not intended to be used directly.

type Internal struct {
	core           *core.Core
	api            *api.API
	infrastructure *infrastructure.Infrastructure
	repositories   *repositories.Repositories
	usecases       *usecases.ApiUsescases
}

func NewInternal() *Internal {
	core := core.Init()
	infrastructure := infrastructure.NewInfrastructure()
	repositories := repositories.NewRepositories(infrastructure)
	usecases := usecases.NewApiUsecases(repositories)
	api := api.NewAPI(usecases)

	return &Internal{
		api:            api,
		core:           core,
		infrastructure: infrastructure,
		repositories:   repositories,
		usecases:       usecases,
	}
}

func (i *Internal) Run() {
	i.api.Run()
}

func (i *Internal) GetCore() *core.Core {
	return i.core
}

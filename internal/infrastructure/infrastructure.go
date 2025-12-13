package infrastructure

import (
	netbridge "github.com/rabbytesoftware/quiver/internal/infrastructure/netbridge"
	"github.com/rabbytesoftware/quiver/internal/infrastructure/requirements"
	"github.com/rabbytesoftware/quiver/internal/infrastructure/runtime"
	"github.com/rabbytesoftware/quiver/internal/infrastructure/translator"
)

type Infrastructure struct {
	Netbridge    netbridge.NetbridgeInterface
	Translator   *translator.Translator
	Requirements requirements.SRVInterface
	Runtime      *runtime.Runtime
}

func NewInfrastructure() *Infrastructure {
	netbridge := netbridge.NewNetbridge()          // Netbridge module
	translator := translator.NewTranslator()       // Translator (ATL & QTL) module
	requirements := requirements.NewRequirements() // Requirements module
	runtimeInstance, err := runtime.New()          // Runtime module
	// Handle runtime initialization error
	if err != nil {
		// Log error and return infrastructure with nil runtime
		// The application can still function without runtime support
		return &Infrastructure{
			Netbridge:    netbridge,
			Translator:   translator,
			Requirements: requirements,
			Runtime:      nil,
		}
	}

	return &Infrastructure{
		Netbridge:    netbridge,
		Translator:   translator,
		Requirements: requirements,
		Runtime:      runtimeInstance,
	}
}

package infrastructure

import (
	netbridge "github.com/rabbytesoftware/quiver/internal/infrastructure/netbridge"
	"github.com/rabbytesoftware/quiver/internal/infrastructure/requirements"

	fns "github.com/rabbytesoftware/quiver/internal/infrastructure/fetchnshare"

	"github.com/rabbytesoftware/quiver/internal/infrastructure/runtime"

	"github.com/rabbytesoftware/quiver/internal/infrastructure/translator"
	tl "github.com/rabbytesoftware/quiver/internal/infrastructure/translator/models"
)

type Infrastructure struct {
	Netbridge    netbridge.NetbridgeInterface
	FNS          fns.FNSInterface
	Translator   tl.TranslatorInterface
	Requirements requirements.SRVInterface
	Runtime      *runtime.Runtime
}

func NewInfrastructure() *Infrastructure {
	netbridge := netbridge.NewNetbridge()          // Netbridge module
	fns := fns.NewFNS()                            // Fetch and Share module
	translator := translator.NewTranslator(fns)    // Translator (ATL & QTL) module
	requirements := requirements.NewRequirements() // Requirements module
	runtimeInstance, err := runtime.New()          // Runtime module

	// Handle runtime initialization error
	if err != nil {
		// Log error and return infrastructure with nil runtime
		// The application can still function without runtime support
		return &Infrastructure{
			Netbridge:    netbridge,
			FNS:          fns,
			Translator:   translator,
			Requirements: requirements,
			Runtime:      nil,
		}
	}

	return &Infrastructure{
		Netbridge:    netbridge,
		FNS:          fns,
		Translator:   translator,
		Requirements: requirements,
		Runtime:      runtimeInstance,
	}
}

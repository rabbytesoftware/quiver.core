package infrastructure

import (
	fns "github.com/rabbytesoftware/quiver/internal/infrastructure/fetchnshare"
	netbridge "github.com/rabbytesoftware/quiver/internal/infrastructure/netbridge"
	"github.com/rabbytesoftware/quiver/internal/infrastructure/requirements"
	"github.com/rabbytesoftware/quiver/internal/infrastructure/runtime"
	"github.com/rabbytesoftware/quiver/internal/infrastructure/translator"
)

type Infrastructure struct {
	Netbridge    netbridge.NetbridgeInterface
	FNS          fns.FNSInterface
	Translator   translator.Translator
	Requirements requirements.SRVInterface
	Runtime      runtime.REEInterface
}

func NewInfrastructure() *Infrastructure {
	netbridge := netbridge.NewNetbridge()          // Netbridge module
	fns := fns.NewFNS()                            // Fetch and Share module
	translator := translator.NewTranslator(fns)    // Translator (ATL & QTL) module
	requirements := requirements.NewRequirements() // Requirements module
	runtime := runtime.NewRuntime()                // Runtime module

	return &Infrastructure{
		Netbridge:    netbridge,
		FNS:          fns,
		Translator:   translator,
		Requirements: requirements,
		Runtime:      runtime,
	}
}

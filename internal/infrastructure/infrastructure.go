package infrastructure

import (
	netbridge "github.com/rabbytesoftware/quiver/internal/infrastructure/netbridge"
	"github.com/rabbytesoftware/quiver/internal/infrastructure/requirements"
	"github.com/rabbytesoftware/quiver/internal/infrastructure/runtime"
	"github.com/rabbytesoftware/quiver/internal/infrastructure/translator"
	"github.com/rabbytesoftware/quiver/internal/infrastructure/wizard"
)

type Infrastructure struct {
	Netbridge    netbridge.NetbridgeInterface
	Translator   *translator.Translator
	Requirements requirements.SRVInterface
	Runtime      *runtime.Runtime
	Wizard       *wizard.Wizard
}

func NewInfrastructure() *Infrastructure {
	netbridge := netbridge.NewNetbridge()          // Netbridge module
	translator := translator.NewTranslator()       // Translator (ATL & QTL) module
	requirements := requirements.NewRequirements() // Requirements module
	runtimeInstance, err := runtime.New()          // Runtime module
	if err != nil {
		panic(err)
	}

	wizardInstance := wizard.NewWizard()

	return &Infrastructure{
		Netbridge:    netbridge,
		Translator:   translator,
		Requirements: requirements,
		Runtime:      runtimeInstance,
		Wizard:       wizardInstance,
	}
}

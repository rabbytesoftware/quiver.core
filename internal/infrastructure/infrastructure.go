package infrastructure

import (
	fns "github.com/rabbytesoftware/quiver/internal/infrastructure/fetchnshare"
	netbridge "github.com/rabbytesoftware/quiver/internal/infrastructure/netbridge"
	"github.com/rabbytesoftware/quiver/internal/infrastructure/requirements"
	"github.com/rabbytesoftware/quiver/internal/infrastructure/runtime"
	"github.com/rabbytesoftware/quiver/internal/infrastructure/translator"
	tl "github.com/rabbytesoftware/quiver/internal/infrastructure/translator/models"
	"github.com/rabbytesoftware/quiver/internal/infrastructure/wizard"
)

type Infrastructure struct {
	Netbridge    netbridge.NetbridgeInterface
	FNS          fns.FNSInterface
	Translator   tl.TranslatorInterface
	Requirements requirements.SRVInterface
	Runtime      *runtime.Runtime
	Wizard       *wizard.Wizard
}

func NewInfrastructure() *Infrastructure {
	netbridge := netbridge.NewNetbridge()
	fns := fns.NewFNS()
	translator := translator.NewTranslator(fns)
	requirements := requirements.NewRequirements()
	runtimeInstance, err := runtime.New()
	if err != nil {
		panic(err)
	}

	wizardInstance := wizard.NewWizard()

	return &Infrastructure{
		Netbridge:    netbridge,
		FNS:          fns,
		Translator:   translator,
		Requirements: requirements,
		Runtime:      runtimeInstance,
		Wizard:       wizardInstance,
	}
}

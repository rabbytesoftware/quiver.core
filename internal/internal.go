package internal

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/engine/manifold/translator"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge"
	"github.com/rabbytesoftware/quiver/internal/engine/requirements"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard/runtime"
)

// Infrastructure holds all engine-level service instances.
type Infrastructure struct {
	Netbridge    netbridge.Netbridge
	Translator   *translator.Translator
	Requirements requirements.SRVInterface
	Runtime      *runtime.Runtime
	Wizard       *wizard.Wizard
}

// NewInfrastructure constructs and wires all engine services.
func NewInfrastructure() *Infrastructure {
	nb, err := netbridge.NewBuilder().Build(context.Background())
	if err != nil {
		panic(err)
	}

	translator := translator.NewTranslator()
	requirements := requirements.NewRequirements()
	runtimeInstance, err := runtime.New()
	if err != nil {
		panic(err)
	}

	wizardInstance := wizard.NewWizard()

	return &Infrastructure{
		Netbridge:    nb,
		Translator:   translator,
		Requirements: requirements,
		Runtime:      runtimeInstance,
		Wizard:       wizardInstance,
	}
}

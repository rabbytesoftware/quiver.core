package internal

import (
	stdruntime "runtime"
	"time"

	"github.com/rabbytesoftware/quiver/internal/engine/manifold/translator"
	netbridge "github.com/rabbytesoftware/quiver/internal/engine/netbridge"
	"github.com/rabbytesoftware/quiver/internal/engine/requirements"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard/runtime"
)

const vaultTTL = 24 * time.Hour

type Infrastructure struct {
	Netbridge    netbridge.NetbridgeInterface
	Translator   *translator.Translator
	Requirements requirements.SRVInterface
	Runtime      *runtime.Runtime
	Wizard       *wizard.Wizard
	Vault        vault.Vault
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

	vaultInstance := vault.New(
		vaultTTL,
		stdruntime.GOOS,
	)

	return &Infrastructure{
		Netbridge:    netbridge,
		Translator:   translator,
		Requirements: requirements,
		Runtime:      runtimeInstance,
		Wizard:       wizardInstance,
		Vault:        vaultInstance,
	}
}

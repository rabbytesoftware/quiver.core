package internal

import (
	stdruntime "runtime"

	"github.com/rabbytesoftware/quiver/internal/engine/manifold/translator"
	netbridge "github.com/rabbytesoftware/quiver/internal/engine/netbridge"
	"github.com/rabbytesoftware/quiver/internal/engine/requirements"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard"
)

type Infrastructure struct {
	Netbridge    netbridge.Netbridge
	Translator   translator.Translator
	Requirements requirements.SRVInterface
	Wizard       wizard.Wizard
	Vault        vault.Vault
}

func NewInfrastructure() *Infrastructure {
	requirements := requirements.New()
	wizardInstance, err := wizard.New()
	if err != nil {
		panic(err)
	}
	vaultInstance := vault.New(
		stdruntime.GOOS,
	)

	return &Infrastructure{
		Requirements: requirements,
		Wizard:       wizardInstance,
		Vault:        vaultInstance,
	}
}

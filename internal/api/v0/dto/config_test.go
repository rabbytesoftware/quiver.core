package dto_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
	"github.com/rabbytesoftware/quiver.core/internal/core/config"
)

func viewFixture() usecases.ConfigView {
	configured := config.Defaults()
	configured.Vault.TTL = "48h"
	configured.API.Host = "tcp://127.0.0.1:9000"

	return usecases.ConfigView{
		Running:         config.Defaults(),
		Configured:      configured,
		Defaults:        config.Defaults(),
		RestartRequired: []string{"vault.ttl"},
	}
}

func TestConfigDTOFrom_CarriesEveryDocument(t *testing.T) {
	got := dto.ConfigDTOFrom(viewFixture())

	assert.Equal(t, config.Defaults().Vault.TTL, got.Running.Vault.TTL)
	assert.Equal(t, "48h", got.Configured.Vault.TTL)
	assert.Equal(t, config.Defaults().Vault.TTL, got.Defaults.Vault.TTL)
	assert.Equal(t, []string{"vault.ttl"}, got.RestartRequired)
}

func TestConfigDTOFrom_ConfiguredAndDefaultsCarryHost(t *testing.T) {
	got := dto.ConfigDTOFrom(viewFixture())

	assert.Equal(t, "tcp://127.0.0.1:9000", got.Configured.API.Host)
	assert.Equal(t, config.Defaults().API.Host, got.Defaults.API.Host)
}

func TestConfigDTOFrom_RunningHasNoAPISection(t *testing.T) {
	raw, err := json.Marshal(dto.ConfigDTOFrom(viewFixture()).Running)
	require.NoError(t, err)

	assert.NotContains(t, string(raw), `"api"`)
	assert.Contains(t, string(raw), `"netbridge"`)
	assert.Contains(t, string(raw), `"search"`)
}

func TestConfigDTOFrom_NilRestartRequiredBecomesEmptyArray(t *testing.T) {
	view := viewFixture()
	view.RestartRequired = nil

	raw, err := json.Marshal(dto.ConfigDTOFrom(view))
	require.NoError(t, err)

	assert.Contains(t, string(raw), `"restart_required":[]`)
}

func TestConfigDTOFrom_MapsEverySection(t *testing.T) {
	configured := config.ConfigData{
		Netbridge: config.Netbridge{Enabled: false, EphemeralPortStart: 1, EphemeralPortEnd: 2},
		API:       config.API{Host: "unix://"},
		Logger:    config.Logger{Enabled: false, Level: "debug"},
		Manifold:  config.Manifold{FetchTimeout: "45s"},
		Vault:     config.Vault{SweepInterval: "10m", TTL: "48h", IndexTTL: "360h"},
		Arrows:    config.Arrows{AutoRetry: config.ArrowAutoRetry{Enabled: false, Retries: 7}},
		Search:    config.Search{PerProviderLimit: 10, FetchConcurrency: 4, ProviderTimeout: "20s"},
	}

	got := dto.ConfigDTOFrom(usecases.ConfigView{
		Running:    configured,
		Configured: configured,
		Defaults:   configured,
	})

	assert.False(t, got.Configured.Netbridge.Enabled)
	assert.Equal(t, 1, got.Configured.Netbridge.EphemeralPortStart)
	assert.Equal(t, 2, got.Configured.Netbridge.EphemeralPortEnd)
	assert.Equal(t, "debug", got.Configured.Logger.Level)
	assert.Equal(t, "45s", got.Configured.Manifold.FetchTimeout)
	assert.Equal(t, "10m", got.Configured.Vault.SweepInterval)
	assert.Equal(t, "360h", got.Configured.Vault.IndexTTL)
	assert.Equal(t, 7, got.Configured.Arrows.AutoRetry.Retries)
	assert.Equal(t, 10, got.Configured.Search.PerProviderLimit)
	assert.Equal(t, "20s", got.Configured.Search.ProviderTimeout)

	assert.Equal(t, 7, got.Defaults.Arrows.AutoRetry.Retries)
	assert.Equal(t, 4, got.Running.Search.FetchConcurrency)
	assert.Equal(t, "360h", got.Running.Vault.IndexTTL)
	assert.False(t, got.Running.Arrows.AutoRetry.Enabled)
	assert.False(t, got.Running.Logger.Enabled)
	assert.Equal(t, "45s", got.Running.Manifold.FetchTimeout)
}

func TestConfigPatchResultDTOFrom_MapsAppliedAndRejected(t *testing.T) {
	got := dto.ConfigPatchResultDTOFrom(usecases.PatchResult{
		Applied:  []string{"vault.ttl"},
		Rejected: []config.FieldError{{Key: "logger.level", Message: "must be one of"}},
	})

	assert.Equal(t, []string{"vault.ttl"}, got.Applied)
	require.Len(t, got.Rejected, 1)
	assert.Equal(t, "logger.level", got.Rejected[0].Key)
	assert.Equal(t, "must be one of", got.Rejected[0].Message)
}

func TestConfigPatchResultDTOFrom_EmptyResultSerialisesAsArrays(t *testing.T) {
	raw, err := json.Marshal(dto.ConfigPatchResultDTOFrom(usecases.PatchResult{}))
	require.NoError(t, err)

	assert.JSONEq(t, `{"applied":[],"rejected":[]}`, string(raw))
}

func TestConfigDTOFrom_CarriesCorrectedSettings(t *testing.T) {
	view := viewFixture()
	view.Corrected = []config.FieldError{{Key: "vault.ttl", Message: "was bad"}}

	got := dto.ConfigDTOFrom(view)

	require.Len(t, got.Corrected, 1)
	assert.Equal(t, "vault.ttl", got.Corrected[0].Key)
	assert.Equal(t, "was bad", got.Corrected[0].Message)
}

func TestConfigDTOFrom_NoCorrectionsSerialisesAsArray(t *testing.T) {
	raw, err := json.Marshal(dto.ConfigDTOFrom(viewFixture()))
	require.NoError(t, err)

	assert.Contains(t, string(raw), `"corrected":[]`)
}

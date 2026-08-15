package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate_DefaultsAreValid(t *testing.T) {
	assert.Empty(t, Validate(Defaults()))
}

func TestValidate_ReportsOffendingKey(t *testing.T) {
	testCases := []struct {
		name    string
		corrupt func(*ConfigData)
		wantKey string
	}{
		{
			name:    "port start out of range",
			corrupt: func(c *ConfigData) { c.Netbridge.EphemeralPortStart = 0 },
			wantKey: "netbridge.ephemeral_port_start",
		},
		{
			name:    "port end out of range",
			corrupt: func(c *ConfigData) { c.Netbridge.EphemeralPortEnd = 70000 },
			wantKey: "netbridge.ephemeral_port_end",
		},
		{
			name:    "host scheme unsupported",
			corrupt: func(c *ConfigData) { c.API.Host = "http://nope" },
			wantKey: "api.host",
		},
		{
			name:    "log level unknown",
			corrupt: func(c *ConfigData) { c.Logger.Level = "waarn" },
			wantKey: "logger.level",
		},
		{
			name:    "fetch timeout unparseable",
			corrupt: func(c *ConfigData) { c.Manifold.FetchTimeout = "banana" },
			wantKey: "manifold.fetch_timeout",
		},
		{
			name:    "sweep interval zero",
			corrupt: func(c *ConfigData) { c.Vault.SweepInterval = "0s" },
			wantKey: "vault.sweep_interval",
		},
		{
			name:    "ttl negative",
			corrupt: func(c *ConfigData) { c.Vault.TTL = "-1h" },
			wantKey: "vault.ttl",
		},
		{
			name:    "index ttl empty",
			corrupt: func(c *ConfigData) { c.Vault.IndexTTL = "" },
			wantKey: "vault.index_ttl",
		},
		{
			name:    "retries negative",
			corrupt: func(c *ConfigData) { c.Arrows.AutoRetry.Retries = -1 },
			wantKey: "arrows.auto_retry.retries",
		},
		{
			name:    "per provider limit zero",
			corrupt: func(c *ConfigData) { c.Search.PerProviderLimit = 0 },
			wantKey: "search.per_provider_limit",
		},
		{
			name:    "fetch concurrency zero",
			corrupt: func(c *ConfigData) { c.Search.FetchConcurrency = 0 },
			wantKey: "search.fetch_concurrency",
		},
		{
			name:    "provider timeout unparseable",
			corrupt: func(c *ConfigData) { c.Search.ProviderTimeout = "soon" },
			wantKey: "search.provider_timeout",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			data := Defaults()
			tc.corrupt(&data)

			errs := Validate(data)
			require.Len(t, errs, 1)
			assert.Equal(t, tc.wantKey, errs[0].Key)
			assert.NotEmpty(t, errs[0].Message)
		})
	}
}

func TestValidate_PortRangeInverted(t *testing.T) {
	data := Defaults()
	data.Netbridge.EphemeralPortStart = 50000
	data.Netbridge.EphemeralPortEnd = 40000

	errs := Validate(data)
	require.Len(t, errs, 1)
	assert.Equal(t, "netbridge.ephemeral_port_end", errs[0].Key)
}

func TestValidate_BooleansNeverReported(t *testing.T) {
	data := Defaults()
	data.Netbridge.Enabled = false
	data.Logger.Enabled = false
	data.Arrows.AutoRetry.Enabled = false

	assert.Empty(t, Validate(data))
}

func TestSanitize_RestoresDefaultForInvalidField(t *testing.T) {
	data := Defaults()
	data.Vault.TTL = "banana"
	data.API.Host = "tcp://127.0.0.1:9000"

	corrected := Sanitize(&data)

	require.Len(t, corrected, 1)
	assert.Equal(t, "vault.ttl", corrected[0].Key)
	assert.Equal(t, Defaults().Vault.TTL, data.Vault.TTL)
	assert.Equal(t, "tcp://127.0.0.1:9000", data.API.Host)
}

func TestSanitize_ValidConfigUnchanged(t *testing.T) {
	data := Defaults()
	before := data

	assert.Empty(t, Sanitize(&data))
	assert.Equal(t, before, data)
}

func TestSanitize_RestoresEveryKnownKey(t *testing.T) {
	data := ConfigData{}

	corrected := Sanitize(&data)

	assert.NotEmpty(t, corrected)
	assert.Empty(t, Validate(data))
}

func TestSanitize_RestoresNegativeRetries(t *testing.T) {
	data := Defaults()
	data.Arrows.AutoRetry.Retries = -1

	corrected := Sanitize(&data)

	require.Len(t, corrected, 1)
	assert.Equal(t, "arrows.auto_retry.retries", corrected[0].Key)
	assert.Equal(t, Defaults().Arrows.AutoRetry.Retries, data.Arrows.AutoRetry.Retries)
}

func TestGetArrows_ReturnsAutoRetrySection(t *testing.T) {
	assert.GreaterOrEqual(t, GetArrows().AutoRetry.Retries, 0)
}

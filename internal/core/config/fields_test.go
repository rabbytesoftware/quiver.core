package config

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func changedConfig() ConfigData {
	return ConfigData{
		Netbridge: Netbridge{Enabled: false, EphemeralPortStart: 50000, EphemeralPortEnd: 60000},
		API:       API{Host: "tcp://127.0.0.1:1"},
		Logger:    Logger{Enabled: false, Level: "debug"},
		Manifold:  Manifold{FetchTimeout: "1s"},
		Vault:     Vault{SweepInterval: "1s", TTL: "1s", IndexTTL: "1s"},
		Arrows:    Arrows{AutoRetry: ArrowAutoRetry{Enabled: false, Retries: 99}},
		Search:    Search{PerProviderLimit: 1, FetchConcurrency: 1, ProviderTimeout: "1s"},
	}
}

func TestKeys_CoverEveryDocumentedSetting(t *testing.T) {
	assert.ElementsMatch(t, []string{
		"netbridge.enabled",
		"netbridge.ephemeral_port_start",
		"netbridge.ephemeral_port_end",
		"api.host",
		"logger.enabled",
		"logger.level",
		"manifold.fetch_timeout",
		"vault.sweep_interval",
		"vault.ttl",
		"vault.index_ttl",
		"arrows.auto_retry.enabled",
		"arrows.auto_retry.retries",
		"search.per_provider_limit",
		"search.fetch_concurrency",
		"search.provider_timeout",
	}, Keys())
}

func TestDiffering_ObservesEveryField(t *testing.T) {
	assert.ElementsMatch(t, Keys(), Differing(Defaults(), changedConfig()))
}

func TestDiffering_IdenticalConfigsYieldNothing(t *testing.T) {
	assert.Empty(t, Differing(Defaults(), Defaults()))
}

func TestRestoreField_IsPerField(t *testing.T) {
	for _, key := range Keys() {
		t.Run(key, func(t *testing.T) {
			data := changedConfig()
			RestoreField(&data, Defaults(), key)

			assert.Equal(t, []string{key}, Differing(data, changedConfig()))
		})
	}
}

func TestRestoreField_UnknownKeyIsIgnored(t *testing.T) {
	data := changedConfig()
	RestoreField(&data, Defaults(), "not.a.real.key")

	assert.Equal(t, changedConfig(), data)
}

// Every rule lives in a validate tag, and an unregistered tag makes the
// validator panic on first use. Validating the defaults therefore proves both
// that the shipped configuration is usable and that no tag is misspelled.
func TestValidate_DefaultsAreValid(t *testing.T) {
	assert.Empty(t, Validate(Defaults()))
}

func TestSetField_DecodesEveryType(t *testing.T) {
	testCases := []struct {
		key  string
		raw  string
		want func(ConfigData) bool
	}{
		{
			key:  "netbridge.enabled",
			raw:  "false",
			want: func(c ConfigData) bool { return !c.Netbridge.Enabled },
		},
		{
			key:  "netbridge.ephemeral_port_start",
			raw:  "50000",
			want: func(c ConfigData) bool { return c.Netbridge.EphemeralPortStart == 50000 },
		},
		{
			key:  "logger.level",
			raw:  `"debug"`,
			want: func(c ConfigData) bool { return c.Logger.Level == "debug" },
		},
		{
			key:  "arrows.auto_retry.retries",
			raw:  "9",
			want: func(c ConfigData) bool { return c.Arrows.AutoRetry.Retries == 9 },
		},
	}

	for _, tc := range testCases {
		t.Run(tc.key, func(t *testing.T) {
			data := Defaults()
			require.NoError(t, SetField(&data, Defaults(), tc.key, json.RawMessage(tc.raw)))
			assert.True(t, tc.want(data))
		})
	}
}

func TestSetField_NullRestoresDefault(t *testing.T) {
	data := changedConfig()

	require.NoError(t, SetField(&data, Defaults(), "vault.ttl", json.RawMessage("null")))

	assert.Equal(t, Defaults().Vault.TTL, data.Vault.TTL)
	assert.Equal(t, changedConfig().Logger.Level, data.Logger.Level)
}

func TestSetField_UnknownKeyReturnsError(t *testing.T) {
	data := Defaults()

	err := SetField(&data, Defaults(), "netbrige.enabled", json.RawMessage("true"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown setting")
}

func TestSetField_WrongTypeReturnsError(t *testing.T) {
	data := Defaults()

	err := SetField(&data, Defaults(), "netbridge.ephemeral_port_start", json.RawMessage(`"abc"`))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a")
}

func TestValidate_ReportsOffendingKey(t *testing.T) {
	testCases := []struct {
		name    string
		corrupt func(*ConfigData)
		wantKey string
	}{
		{"port start", func(c *ConfigData) { c.Netbridge.EphemeralPortStart = 0 }, "netbridge.ephemeral_port_start"},
		{"port end", func(c *ConfigData) { c.Netbridge.EphemeralPortEnd = 70000 }, "netbridge.ephemeral_port_end"},
		{"host", func(c *ConfigData) { c.API.Host = "http://nope" }, "api.host"},
		{"level", func(c *ConfigData) { c.Logger.Level = "waarn" }, "logger.level"},
		{"fetch timeout", func(c *ConfigData) { c.Manifold.FetchTimeout = "banana" }, "manifold.fetch_timeout"},
		{"sweep interval", func(c *ConfigData) { c.Vault.SweepInterval = "0s" }, "vault.sweep_interval"},
		{"ttl", func(c *ConfigData) { c.Vault.TTL = "-1h" }, "vault.ttl"},
		{"index ttl", func(c *ConfigData) { c.Vault.IndexTTL = "" }, "vault.index_ttl"},
		{"retries", func(c *ConfigData) { c.Arrows.AutoRetry.Retries = -1 }, "arrows.auto_retry.retries"},
		{"per provider limit", func(c *ConfigData) { c.Search.PerProviderLimit = 0 }, "search.per_provider_limit"},
		{"fetch concurrency", func(c *ConfigData) { c.Search.FetchConcurrency = 0 }, "search.fetch_concurrency"},
		{"provider timeout", func(c *ConfigData) { c.Search.ProviderTimeout = "soon" }, "search.provider_timeout"},
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

// Every non-struct field of ConfigData must be reachable through Keys. A field
// of an unsupported kind is skipped by walk, so it would validate, patch and
// persist as though it did not exist; this fails the moment one is added.
func TestKeys_ReachEveryLeafOfConfigData(t *testing.T) {
	var leaves []string
	collectLeaves(reflect.TypeOf(ConfigData{}), "", &leaves)

	assert.ElementsMatch(t, leaves, Keys(),
		"a ConfigData field is not reachable through Keys; is its type one of bool, int or string?")
}

func collectLeaves(t reflect.Type, prefix string, out *[]string) {
	for i := range t.NumField() {
		f := t.Field(i)

		name, _, _ := strings.Cut(f.Tag.Get("yaml"), ",")
		if name == "" || name == "-" {
			continue
		}

		if prefix != "" {
			name = prefix + "." + name
		}

		if f.Type.Kind() == reflect.Struct {
			collectLeaves(f.Type, name, out)
			continue
		}

		*out = append(*out, name)
	}
}

package config

import (
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

// Guards the table against a forgotten entry. Every configuration field must
// be reachable through Fields, or validation would silently skip it and
// RestoreField would silently do nothing.
func TestFields_ObserveEveryField(t *testing.T) {
	changed := changedConfig()

	seen := make(map[string]bool)
	for _, f := range Fields() {
		assert.False(t, seen[f.Key()], "duplicate key %s", f.Key())
		seen[f.Key()] = true
		assert.True(t, f.Differs(Defaults(), changed), "%s does not observe a change", f.Key())
	}
}

func TestFields_RestoreIsPerField(t *testing.T) {
	for _, f := range Fields() {
		t.Run(f.Key(), func(t *testing.T) {
			data := changedConfig()
			f.Restore(&data, Defaults())

			assert.False(t, f.Differs(data, Defaults()), "%s was not restored", f.Key())

			for _, other := range Fields() {
				if other.Key() == f.Key() {
					continue
				}
				assert.True(t, other.Differs(data, Defaults()),
					"restoring %s also changed %s", f.Key(), other.Key())
			}
		})
	}
}

func TestFields_DefaultsPassEveryCheck(t *testing.T) {
	for _, f := range Fields() {
		assert.Nil(t, f.Check(Defaults()), "%s rejects its own default", f.Key())
	}
}

func TestKeys_MatchesFieldsInOrder(t *testing.T) {
	fields := Fields()
	keys := Keys()

	require.Len(t, keys, len(fields))
	for i, f := range fields {
		assert.Equal(t, f.Key(), keys[i])
	}
}

func TestKeys_CoversEveryDocumentedSetting(t *testing.T) {
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

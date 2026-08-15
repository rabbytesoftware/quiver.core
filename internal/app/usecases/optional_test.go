package usecases

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type optionalHolder struct {
	Flag  Optional[bool]   `json:"flag"`
	Count Optional[int]    `json:"count"`
	Label Optional[string] `json:"label"`
}

func TestOptional_AbsentFieldIsNotSet(t *testing.T) {
	var holder optionalHolder
	require.NoError(t, json.Unmarshal([]byte(`{}`), &holder))

	assert.False(t, holder.Flag.IsSet())
	assert.False(t, holder.Count.IsSet())
	assert.False(t, holder.Label.IsSet())
	assert.False(t, holder.Count.IsReset())
}

func TestOptional_ExplicitNullIsReset(t *testing.T) {
	var holder optionalHolder
	require.NoError(t, json.Unmarshal([]byte(`{"count":null}`), &holder))

	assert.True(t, holder.Count.IsSet())
	assert.True(t, holder.Count.IsReset())
	assert.Equal(t, 0, holder.Count.Value())
}

func TestOptional_ValueIsDecoded(t *testing.T) {
	var holder optionalHolder
	require.NoError(t, json.Unmarshal(
		[]byte(`{"flag":true,"count":50000,"label":"debug"}`), &holder,
	))

	assert.True(t, holder.Flag.IsSet())
	assert.False(t, holder.Flag.IsReset())
	assert.True(t, holder.Flag.Value())

	assert.Equal(t, 50000, holder.Count.Value())
	assert.Equal(t, "debug", holder.Label.Value())
}

func TestOptional_FalseIsDistinguishableFromAbsent(t *testing.T) {
	var holder optionalHolder
	require.NoError(t, json.Unmarshal([]byte(`{"flag":false}`), &holder))

	assert.True(t, holder.Flag.IsSet())
	assert.False(t, holder.Flag.IsReset())
	assert.False(t, holder.Flag.Value())
}

func TestOptional_WrongTypeReturnsError(t *testing.T) {
	var holder optionalHolder
	err := json.Unmarshal([]byte(`{"count":"not-a-number"}`), &holder)

	require.Error(t, err)
}

func TestConfigPatch_DecodesNestedSections(t *testing.T) {
	var patch ConfigPatch
	require.NoError(t, json.Unmarshal([]byte(`{
		"netbridge": {"ephemeral_port_start": 50000},
		"logger":    {"level": "debug"},
		"vault":     {"ttl": null},
		"arrows":    {"auto_retry": {"retries": 5}}
	}`), &patch))

	assert.True(t, patch.Netbridge.EphemeralPortStart.IsSet())
	assert.Equal(t, 50000, patch.Netbridge.EphemeralPortStart.Value())
	assert.False(t, patch.Netbridge.EphemeralPortEnd.IsSet())
	assert.Equal(t, "debug", patch.Logger.Level.Value())
	assert.True(t, patch.Vault.TTL.IsReset())
	assert.Equal(t, 5, patch.Arrows.AutoRetry.Retries.Value())
	assert.False(t, patch.API.Host.IsSet())
}

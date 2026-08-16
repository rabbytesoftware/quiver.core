package config_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	repoconfig "github.com/rabbytesoftware/quiver.core/internal/app/repositories/config"
	coreconfig "github.com/rabbytesoftware/quiver.core/internal/core/config"
)

// The repository resolves its path from the user's home directory, and home
// resolution is not redirectable on every platform: the windows implementation
// substitutes the OS username into a template and consults no environment
// variable. Writing through it here would mutate a real config file that
// internal/core/config and internal/engine/vault read in their own test
// binaries. Round-tripping to disk is covered in internal/core/config against
// a temp path instead.
func TestNew_ReadsWithoutWriting(t *testing.T) {
	repo := repoconfig.New()

	assert.Equal(t, coreconfig.Defaults(), repo.Defaults())
	assert.NotEmpty(t, repo.Running().API.Host)
	assert.Empty(t, repo.Validate(coreconfig.Defaults()))

	got, err := repo.Configured()
	require.NoError(t, err)
	assert.Empty(t, repo.Validate(got))
}

func TestNew_ValidateReportsInvalidField(t *testing.T) {
	data := coreconfig.Defaults()
	data.Logger.Level = "banana"

	errs := repoconfig.New().Validate(data)

	require.Len(t, errs, 1)
	assert.Equal(t, "logger.level", errs[0].Key)
}

func TestDiffering_ReportsChangedKeys(t *testing.T) {
	changed := coreconfig.Defaults()
	changed.Vault.TTL = "72h"

	assert.Equal(t, []string{"vault.ttl"},
		repoconfig.Differing(coreconfig.Defaults(), changed))
}

func TestRestoreField_CopiesOneFieldFromSource(t *testing.T) {
	data := coreconfig.Defaults()
	data.Vault.TTL = "999h"
	data.Logger.Level = "debug"

	repoconfig.RestoreField(&data, coreconfig.Defaults(), "vault.ttl")

	assert.Equal(t, coreconfig.Defaults().Vault.TTL, data.Vault.TTL)
	assert.Equal(t, "debug", data.Logger.Level)
}

func TestRestoreField_UnknownKeyIsIgnored(t *testing.T) {
	data := coreconfig.Defaults()
	data.Vault.TTL = "999h"

	repoconfig.RestoreField(&data, coreconfig.Defaults(), "not.a.real.key")

	assert.Equal(t, "999h", data.Vault.TTL)
}

func TestSetField_DecodesValue(t *testing.T) {
	data := coreconfig.Defaults()

	require.NoError(t, repoconfig.SetField(
		&data, coreconfig.Defaults(), "vault.ttl", json.RawMessage(`"72h"`),
	))

	assert.Equal(t, "72h", data.Vault.TTL)
}

func TestSetField_NullRestoresDefault(t *testing.T) {
	data := coreconfig.Defaults()
	data.Vault.TTL = "999h"

	require.NoError(t, repoconfig.SetField(
		&data, coreconfig.Defaults(), "vault.ttl", json.RawMessage("null"),
	))

	assert.Equal(t, coreconfig.Defaults().Vault.TTL, data.Vault.TTL)
}

func TestSetField_UnknownKeyReturnsError(t *testing.T) {
	data := coreconfig.Defaults()

	err := repoconfig.SetField(
		&data, coreconfig.Defaults(), "nope.nope", json.RawMessage("1"),
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown setting")
}

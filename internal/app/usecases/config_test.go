package usecases

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/core/config"
	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
)

type stubConfigStore struct {
	running    config.ConfigData
	configured config.ConfigData
	saved      []config.ConfigData
	saveErr    error
	loadErr    error
}

func newStubConfigStore() *stubConfigStore {
	return &stubConfigStore{
		running:    config.Defaults(),
		configured: config.Defaults(),
	}
}

func (s *stubConfigStore) Running() config.ConfigData {
	return s.running
}

func (s *stubConfigStore) Configured() (config.ConfigData, error) {
	if s.loadErr != nil {
		return config.ConfigData{}, s.loadErr
	}
	return s.configured, nil
}

func (s *stubConfigStore) Defaults() config.ConfigData {
	return config.Defaults()
}

func (s *stubConfigStore) Validate(data config.ConfigData) []config.FieldError {
	return config.Validate(data)
}

func (s *stubConfigStore) Save(data config.ConfigData) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.saved = append(s.saved, data)
	return nil
}

func TestConfigUsecase_Get_ReturnsThreeDocuments(t *testing.T) {
	store := newStubConfigStore()
	store.configured.Vault.TTL = "48h"

	view, err := NewConfigUsecase(store).Get(context.Background())

	require.NoError(t, err)
	assert.Equal(t, config.Defaults().Vault.TTL, view.Running.Vault.TTL)
	assert.Equal(t, "48h", view.Configured.Vault.TTL)
	assert.Equal(t, config.Defaults().Vault.TTL, view.Defaults.Vault.TTL)
}

func TestConfigUsecase_Get_RestartRequiredListsDifferingKeys(t *testing.T) {
	store := newStubConfigStore()
	store.configured.Vault.TTL = "48h"
	store.configured.Netbridge.EphemeralPortStart = 50000
	store.configured.Logger.Enabled = false

	view, err := NewConfigUsecase(store).Get(context.Background())

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{
		"vault.ttl",
		"netbridge.ephemeral_port_start",
		"logger.enabled",
	}, view.RestartRequired)
}

func TestConfigUsecase_Get_RestartRequiredNeverIncludesHost(t *testing.T) {
	store := newStubConfigStore()
	store.configured.API.Host = "tcp://0.0.0.0:40257"

	view, err := NewConfigUsecase(store).Get(context.Background())

	require.NoError(t, err)
	assert.Empty(t, view.RestartRequired)
}

func TestConfigUsecase_Get_NothingChangedYieldsEmptyRestartRequired(t *testing.T) {
	view, err := NewConfigUsecase(newStubConfigStore()).Get(context.Background())

	require.NoError(t, err)
	assert.Empty(t, view.RestartRequired)
}

func TestConfigUsecase_Get_PropagatesLoadError(t *testing.T) {
	store := newStubConfigStore()
	store.loadErr = errors.New("disk on fire")

	_, err := NewConfigUsecase(store).Get(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "disk on fire")
}

func TestConfigUsecase_Patch_AppliesSingleField(t *testing.T) {
	store := newStubConfigStore()

	var patch ConfigPatch
	patch.Netbridge.EphemeralPortStart = setOptional(50000)

	result, err := NewConfigUsecase(store).Patch(context.Background(), patch)

	require.NoError(t, err)
	assert.Equal(t, []string{"netbridge.ephemeral_port_start"}, result.Applied)
	assert.Empty(t, result.Rejected)
	require.Len(t, store.saved, 1)
	assert.Equal(t, 50000, store.saved[0].Netbridge.EphemeralPortStart)
}

func TestConfigUsecase_Patch_ResetRestoresDefault(t *testing.T) {
	store := newStubConfigStore()
	store.configured.Vault.TTL = "48h"

	var patch ConfigPatch
	patch.Vault.TTL = resetOptional[string]()

	result, err := NewConfigUsecase(store).Patch(context.Background(), patch)

	require.NoError(t, err)
	assert.Equal(t, []string{"vault.ttl"}, result.Applied)
	require.Len(t, store.saved, 1)
	assert.Equal(t, config.Defaults().Vault.TTL, store.saved[0].Vault.TTL)
}

func TestConfigUsecase_Patch_AppliesValidAndRejectsInvalid(t *testing.T) {
	store := newStubConfigStore()

	var patch ConfigPatch
	patch.Netbridge.EphemeralPortStart = setOptional(50000)
	patch.Logger.Level = setOptional("banana")
	patch.Vault.TTL = setOptional("48h")

	result, err := NewConfigUsecase(store).Patch(context.Background(), patch)

	require.NoError(t, err)
	assert.ElementsMatch(t,
		[]string{"netbridge.ephemeral_port_start", "vault.ttl"}, result.Applied)
	require.Len(t, result.Rejected, 1)
	assert.Equal(t, "logger.level", result.Rejected[0].Key)

	require.Len(t, store.saved, 1)
	assert.Equal(t, 50000, store.saved[0].Netbridge.EphemeralPortStart)
	assert.Equal(t, "48h", store.saved[0].Vault.TTL)
	assert.Equal(t, config.Defaults().Logger.Level, store.saved[0].Logger.Level)
}

func TestConfigUsecase_Patch_AllRejectedReturnsInvalidConfig(t *testing.T) {
	store := newStubConfigStore()

	var patch ConfigPatch
	patch.Logger.Level = setOptional("banana")

	_, err := NewConfigUsecase(store).Patch(context.Background(), patch)

	require.ErrorIs(t, err, apperrors.ErrInvalidConfig)
	assert.Contains(t, err.Error(), "logger.level")
	assert.Empty(t, store.saved)
}

func TestConfigUsecase_Patch_EmptyPatchSavesNothing(t *testing.T) {
	store := newStubConfigStore()

	result, err := NewConfigUsecase(store).Patch(context.Background(), ConfigPatch{})

	require.NoError(t, err)
	assert.Empty(t, result.Applied)
	assert.Empty(t, result.Rejected)
	assert.Empty(t, store.saved)
}

func TestConfigUsecase_Patch_CrossFieldRejectsTheTouchedKey(t *testing.T) {
	store := newStubConfigStore()
	store.configured.Netbridge.EphemeralPortStart = 49152
	store.configured.Netbridge.EphemeralPortEnd = 50000

	var patch ConfigPatch
	patch.Netbridge.EphemeralPortStart = setOptional(60000)

	result, err := NewConfigUsecase(store).Patch(context.Background(), patch)

	require.ErrorIs(t, err, apperrors.ErrInvalidConfig)
	assert.Empty(t, result.Applied)
	assert.Empty(t, store.saved)
	assert.Contains(t, err.Error(), "netbridge.ephemeral_port_start")
}

func TestConfigUsecase_Patch_PropagatesSaveError(t *testing.T) {
	store := newStubConfigStore()
	store.saveErr = errors.New("read-only filesystem")

	var patch ConfigPatch
	patch.Vault.TTL = setOptional("48h")

	_, err := NewConfigUsecase(store).Patch(context.Background(), patch)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "read-only filesystem")
}

func TestConfigUsecase_Patch_PropagatesLoadError(t *testing.T) {
	store := newStubConfigStore()
	store.loadErr = errors.New("disk on fire")

	_, err := NewConfigUsecase(store).Patch(context.Background(), ConfigPatch{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "disk on fire")
}

func TestConfigUsecase_Patch_TouchesEverySection(t *testing.T) {
	store := newStubConfigStore()

	var patch ConfigPatch
	patch.Netbridge.Enabled = setOptional(false)
	patch.Netbridge.EphemeralPortStart = setOptional(50000)
	patch.Netbridge.EphemeralPortEnd = setOptional(50500)
	patch.API.Host = setOptional("tcp://127.0.0.1:9000")
	patch.Logger.Enabled = setOptional(false)
	patch.Logger.Level = setOptional("debug")
	patch.Manifold.FetchTimeout = setOptional("45s")
	patch.Vault.SweepInterval = setOptional("10m")
	patch.Vault.TTL = setOptional("48h")
	patch.Vault.IndexTTL = setOptional("360h")
	patch.Arrows.AutoRetry.Enabled = setOptional(false)
	patch.Arrows.AutoRetry.Retries = setOptional(7)
	patch.Search.PerProviderLimit = setOptional(10)
	patch.Search.FetchConcurrency = setOptional(4)
	patch.Search.ProviderTimeout = setOptional("20s")

	result, err := NewConfigUsecase(store).Patch(context.Background(), patch)

	require.NoError(t, err)
	assert.Len(t, result.Applied, 15)
	assert.Empty(t, result.Rejected)

	require.Len(t, store.saved, 1)
	saved := store.saved[0]
	assert.False(t, saved.Netbridge.Enabled)
	assert.Equal(t, "tcp://127.0.0.1:9000", saved.API.Host)
	assert.Equal(t, "debug", saved.Logger.Level)
	assert.Equal(t, "45s", saved.Manifold.FetchTimeout)
	assert.Equal(t, "360h", saved.Vault.IndexTTL)
	assert.Equal(t, 7, saved.Arrows.AutoRetry.Retries)
	assert.Equal(t, 4, saved.Search.FetchConcurrency)
}

func TestConfigUsecase_Get_RunningReportedForEverySection(t *testing.T) {
	store := newStubConfigStore()
	store.running.Search.FetchConcurrency = 3

	view, err := NewConfigUsecase(store).Get(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 3, view.Running.Search.FetchConcurrency)
}

func setOptional[T Leaf](v T) Optional[T] {
	return Optional[T]{set: true, value: v}
}

func resetOptional[T Leaf]() Optional[T] {
	return Optional[T]{set: true, reset: true}
}

func TestBlameKey_AttributesToTouchedField(t *testing.T) {
	testCases := []struct {
		name    string
		key     string
		touched []string
		want    string
	}{
		{
			name:    "reported key was touched",
			key:     "logger.level",
			touched: []string{"logger.level"},
			want:    "logger.level",
		},
		{
			name:    "range broken by raising the start",
			key:     keyPortEnd,
			touched: []string{keyPortStart},
			want:    keyPortStart,
		},
		{
			name:    "range broken by lowering the end",
			key:     keyPortStart,
			touched: []string{keyPortEnd},
			want:    keyPortEnd,
		},
		{
			name:    "pre-existing failure the caller did not cause",
			key:     "vault.ttl",
			touched: []string{"logger.level"},
			want:    "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			index := make(map[string]bool, len(tc.touched))
			for _, key := range tc.touched {
				index[key] = true
			}

			assert.Equal(t, tc.want, blameKey(tc.key, index))
		})
	}
}

func TestConfigUsecase_Patch_IgnoresUntouchedInvalidField(t *testing.T) {
	store := newStubConfigStore()
	store.configured.Logger.Level = "banana"

	var patch ConfigPatch
	patch.Vault.TTL = setOptional("48h")

	result, err := NewConfigUsecase(store).Patch(context.Background(), patch)

	require.NoError(t, err)
	assert.Equal(t, []string{"vault.ttl"}, result.Applied)
	assert.Empty(t, result.Rejected)
	require.Len(t, store.saved, 1)
	assert.Equal(t, "banana", store.saved[0].Logger.Level)
}

func TestCoreConfigStore_ReadsAndWritesRealConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	require.NoError(t, os.MkdirAll(filepath.Dir(metadata.GetConfigPath()), 0o750))

	store := NewCoreConfigStore()

	assert.Equal(t, config.Defaults(), store.Defaults())
	assert.NotEmpty(t, store.Running().API.Host)
	assert.Empty(t, store.Validate(config.Defaults()))

	want := config.Defaults()
	want.Vault.TTL = "96h"
	require.NoError(t, store.Save(want))

	got, err := store.Configured()
	require.NoError(t, err)
	assert.Equal(t, "96h", got.Vault.TTL)
}

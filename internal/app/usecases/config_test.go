package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	repoconfig "github.com/rabbytesoftware/quiver.core/internal/app/repositories/config"
	coreconfig "github.com/rabbytesoftware/quiver.core/internal/core/config"
)

type stubConfigRepo struct {
	running    repoconfig.Data
	configured repoconfig.Data
	saved      []repoconfig.Data
	saveErr    error
	loadErr    error
}

func newStubConfigRepo() *stubConfigRepo {
	return &stubConfigRepo{
		running:    coreconfig.Defaults(),
		configured: coreconfig.Defaults(),
	}
}

func (s *stubConfigRepo) Running() repoconfig.Data {
	return s.running
}

func (s *stubConfigRepo) Configured() (repoconfig.Data, error) {
	if s.loadErr != nil {
		return repoconfig.Data{}, s.loadErr
	}
	return s.configured, nil
}

func (s *stubConfigRepo) Defaults() repoconfig.Data {
	return coreconfig.Defaults()
}

func (s *stubConfigRepo) Validate(data repoconfig.Data) []repoconfig.FieldError {
	return coreconfig.Validate(data)
}

func (s *stubConfigRepo) Save(data repoconfig.Data) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.saved = append(s.saved, data)
	return nil
}

func TestConfigUsecase_Get_ReturnsThreeDocuments(t *testing.T) {
	store := newStubConfigRepo()
	store.configured.Vault.TTL = "48h"

	view, err := NewConfigUsecase(store).Get(context.Background())

	require.NoError(t, err)
	assert.Equal(t, coreconfig.Defaults().Vault.TTL, view.Running.Vault.TTL)
	assert.Equal(t, "48h", view.Configured.Vault.TTL)
	assert.Equal(t, coreconfig.Defaults().Vault.TTL, view.Defaults.Vault.TTL)
}

func TestConfigUsecase_Get_RestartRequiredListsDifferingKeys(t *testing.T) {
	store := newStubConfigRepo()
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
	store := newStubConfigRepo()
	store.configured.API.Host = "tcp://0.0.0.0:40257"

	view, err := NewConfigUsecase(store).Get(context.Background())

	require.NoError(t, err)
	assert.Empty(t, view.RestartRequired)
}

func TestConfigUsecase_Get_NothingChangedYieldsEmptyRestartRequired(t *testing.T) {
	view, err := NewConfigUsecase(newStubConfigRepo()).Get(context.Background())

	require.NoError(t, err)
	assert.Empty(t, view.RestartRequired)
}

func TestConfigUsecase_Get_PropagatesLoadError(t *testing.T) {
	store := newStubConfigRepo()
	store.loadErr = errors.New("disk on fire")

	_, err := NewConfigUsecase(store).Get(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "disk on fire")
}

func TestConfigUsecase_Patch_AppliesSingleField(t *testing.T) {
	store := newStubConfigRepo()

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
	store := newStubConfigRepo()
	store.configured.Vault.TTL = "48h"

	var patch ConfigPatch
	patch.Vault.TTL = resetOptional[string]()

	result, err := NewConfigUsecase(store).Patch(context.Background(), patch)

	require.NoError(t, err)
	assert.Equal(t, []string{"vault.ttl"}, result.Applied)
	require.Len(t, store.saved, 1)
	assert.Equal(t, coreconfig.Defaults().Vault.TTL, store.saved[0].Vault.TTL)
}

func TestConfigUsecase_Patch_AppliesValidAndRejectsInvalid(t *testing.T) {
	store := newStubConfigRepo()

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
	assert.Equal(t, coreconfig.Defaults().Logger.Level, store.saved[0].Logger.Level)
}

func TestConfigUsecase_Patch_AllRejectedReturnsInvalidConfig(t *testing.T) {
	store := newStubConfigRepo()

	var patch ConfigPatch
	patch.Logger.Level = setOptional("banana")

	_, err := NewConfigUsecase(store).Patch(context.Background(), patch)

	require.ErrorIs(t, err, apperrors.ErrInvalidConfig)
	assert.Contains(t, err.Error(), "logger.level")
	assert.Empty(t, store.saved)
}

func TestConfigUsecase_Patch_EmptyPatchSavesNothing(t *testing.T) {
	store := newStubConfigRepo()

	result, err := NewConfigUsecase(store).Patch(context.Background(), ConfigPatch{})

	require.NoError(t, err)
	assert.Empty(t, result.Applied)
	assert.Empty(t, result.Rejected)
	assert.Empty(t, store.saved)
}

func TestConfigUsecase_Patch_CrossFieldRejectsTheTouchedKey(t *testing.T) {
	store := newStubConfigRepo()
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
	store := newStubConfigRepo()
	store.saveErr = errors.New("read-only filesystem")

	var patch ConfigPatch
	patch.Vault.TTL = setOptional("48h")

	_, err := NewConfigUsecase(store).Patch(context.Background(), patch)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "read-only filesystem")
}

func TestConfigUsecase_Patch_PropagatesLoadError(t *testing.T) {
	store := newStubConfigRepo()
	store.loadErr = errors.New("disk on fire")

	_, err := NewConfigUsecase(store).Patch(context.Background(), ConfigPatch{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "disk on fire")
}

func TestConfigUsecase_Patch_TouchesEverySection(t *testing.T) {
	store := newStubConfigRepo()

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
	store := newStubConfigRepo()
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
	store := newStubConfigRepo()
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

// Guards the table against a forgotten entry: every configuration field must
// be reachable through configFields, or a patch would silently fail to apply
// and a pending change would silently fail to report.
func TestConfigFields_CoverEveryField(t *testing.T) {
	changed := repoconfig.Data{
		Netbridge: coreconfig.Netbridge{Enabled: false, EphemeralPortStart: 1, EphemeralPortEnd: 2},
		API:       coreconfig.API{Host: "tcp://127.0.0.1:1"},
		Logger:    coreconfig.Logger{Enabled: false, Level: "debug"},
		Manifold:  coreconfig.Manifold{FetchTimeout: "1s"},
		Vault:     coreconfig.Vault{SweepInterval: "1s", TTL: "1s", IndexTTL: "1s"},
		Arrows:    coreconfig.Arrows{AutoRetry: coreconfig.ArrowAutoRetry{Enabled: false, Retries: 99}},
		Search:    coreconfig.Search{PerProviderLimit: 1, FetchConcurrency: 1, ProviderTimeout: "1s"},
	}

	fields := configFields()

	seen := make(map[string]bool, len(fields))
	for _, f := range fields {
		assert.False(t, seen[f.Key()], "duplicate key %s", f.Key())
		seen[f.Key()] = true
		assert.True(t, f.Differs(coreconfig.Defaults(), changed),
			"%s does not observe a changed value", f.Key())
	}

	// Every field differs, so every field but the excluded host must be pending.
	assert.Len(t, pendingKeys(coreconfig.Defaults(), changed), len(fields)-1)
	assert.NotContains(t, pendingKeys(coreconfig.Defaults(), changed), keyHost)
}

func TestConfigFields_RestoreIsPerField(t *testing.T) {
	data := coreconfig.Defaults()
	data.Vault.TTL = "999h"
	data.Logger.Level = "debug"

	restoreField(&data, coreconfig.Defaults(), "vault.ttl")

	assert.Equal(t, coreconfig.Defaults().Vault.TTL, data.Vault.TTL)
	assert.Equal(t, "debug", data.Logger.Level)
}

func TestConfigFields_RestoreUnknownKeyIsIgnored(t *testing.T) {
	data := coreconfig.Defaults()
	data.Vault.TTL = "999h"

	restoreField(&data, coreconfig.Defaults(), "not.a.real.key")

	assert.Equal(t, "999h", data.Vault.TTL)
}

// The patch table and the core field table are separate because ConfigPatch is
// an app-layer type that core cannot see. They must still describe the same
// surface: a field in one and not the other is a setting that either cannot be
// patched or cannot be validated.
func TestConfigFields_MatchCoreFieldTable(t *testing.T) {
	keys := make([]string, 0, len(configFields()))
	for _, f := range configFields() {
		keys = append(keys, f.Key())
	}

	assert.ElementsMatch(t, coreconfig.Keys(), keys)
}

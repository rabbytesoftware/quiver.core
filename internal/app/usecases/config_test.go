package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
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

func patchWith(t *testing.T, store *stubConfigRepo, body string) (PatchResult, error) {
	t.Helper()
	return NewConfigUsecase(store).Patch(context.Background(), json.RawMessage(body))
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
		"vault.ttl", "netbridge.ephemeral_port_start", "logger.enabled",
	}, view.RestartRequired)
}

func TestConfigUsecase_Get_RestartRequiredNeverIncludesHost(t *testing.T) {
	store := newStubConfigRepo()
	store.configured.API.Host = "tcp://0.0.0.0:40257"

	view, err := NewConfigUsecase(store).Get(context.Background())

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

func TestConfigUsecase_Patch_AppliesSingleSetting(t *testing.T) {
	store := newStubConfigRepo()

	result, err := patchWith(t, store, `{"netbridge":{"ephemeral_port_start":50000}}`)

	require.NoError(t, err)
	assert.Equal(t, []string{"netbridge.ephemeral_port_start"}, result.Applied)
	assert.Empty(t, result.Rejected)
	require.Len(t, store.saved, 1)
	assert.Equal(t, 50000, store.saved[0].Netbridge.EphemeralPortStart)
}

func TestConfigUsecase_Patch_NullRestoresDefault(t *testing.T) {
	store := newStubConfigRepo()
	store.configured.Vault.TTL = "48h"

	result, err := patchWith(t, store, `{"vault":{"ttl":null}}`)

	require.NoError(t, err)
	assert.Equal(t, []string{"vault.ttl"}, result.Applied)
	require.Len(t, store.saved, 1)
	assert.Equal(t, coreconfig.Defaults().Vault.TTL, store.saved[0].Vault.TTL)
}

func TestConfigUsecase_Patch_AppliesValidAndRejectsInvalid(t *testing.T) {
	store := newStubConfigRepo()

	result, err := patchWith(t, store,
		`{"netbridge":{"ephemeral_port_start":50000},"logger":{"level":"banana"},"vault":{"ttl":"48h"}}`)

	require.NoError(t, err)
	assert.ElementsMatch(t,
		[]string{"netbridge.ephemeral_port_start", "vault.ttl"}, result.Applied)
	require.Len(t, result.Rejected, 1)
	assert.Equal(t, "logger.level", result.Rejected[0].Key)

	require.Len(t, store.saved, 1)
	assert.Equal(t, coreconfig.Defaults().Logger.Level, store.saved[0].Logger.Level)
}

func TestConfigUsecase_Patch_UnknownSettingIsRejected(t *testing.T) {
	store := newStubConfigRepo()

	result, err := patchWith(t, store, `{"netbrige":{"enabled":false},"vault":{"ttl":"48h"}}`)

	require.NoError(t, err)
	assert.Equal(t, []string{"vault.ttl"}, result.Applied)
	require.Len(t, result.Rejected, 1)
	assert.Equal(t, "netbrige.enabled", result.Rejected[0].Key)
	assert.Contains(t, result.Rejected[0].Message, "unknown setting")
}

func TestConfigUsecase_Patch_WrongTypeIsRejectedPerSetting(t *testing.T) {
	store := newStubConfigRepo()

	result, err := patchWith(t, store,
		`{"netbridge":{"ephemeral_port_start":"abc"},"vault":{"ttl":"48h"}}`)

	require.NoError(t, err)
	assert.Equal(t, []string{"vault.ttl"}, result.Applied)
	require.Len(t, result.Rejected, 1)
	assert.Equal(t, "netbridge.ephemeral_port_start", result.Rejected[0].Key)
}

func TestConfigUsecase_Patch_AllRejectedReturnsInvalidConfig(t *testing.T) {
	store := newStubConfigRepo()

	_, err := patchWith(t, store, `{"logger":{"level":"banana"}}`)

	require.ErrorIs(t, err, apperrors.ErrInvalidConfig)
	assert.Contains(t, err.Error(), "logger.level")
	assert.Empty(t, store.saved)
}

func TestConfigUsecase_Patch_EmptyBodySavesNothing(t *testing.T) {
	store := newStubConfigRepo()

	result, err := patchWith(t, store, `{}`)

	require.NoError(t, err)
	assert.Empty(t, result.Applied)
	assert.Empty(t, result.Rejected)
	assert.Empty(t, store.saved)
}

func TestConfigUsecase_Patch_EmptySectionSavesNothing(t *testing.T) {
	store := newStubConfigRepo()

	result, err := patchWith(t, store, `{"vault":{}}`)

	require.NoError(t, err)
	assert.Empty(t, result.Applied)
	assert.Empty(t, store.saved)
}

func TestConfigUsecase_Patch_NonObjectBodyIsInvalidConfig(t *testing.T) {
	store := newStubConfigRepo()

	_, err := patchWith(t, store, `["nope"]`)

	require.ErrorIs(t, err, apperrors.ErrInvalidConfig)
	assert.Empty(t, store.saved)
}

func TestConfigUsecase_Patch_CrossFieldRejectsTheTouchedSetting(t *testing.T) {
	store := newStubConfigRepo()
	store.configured.Netbridge.EphemeralPortStart = 49152
	store.configured.Netbridge.EphemeralPortEnd = 50000

	result, err := patchWith(t, store, `{"netbridge":{"ephemeral_port_start":60000}}`)

	require.ErrorIs(t, err, apperrors.ErrInvalidConfig)
	assert.Empty(t, result.Applied)
	assert.Empty(t, store.saved)
	assert.Contains(t, err.Error(), "netbridge.ephemeral_port_start")
}

func TestConfigUsecase_Patch_PropagatesSaveError(t *testing.T) {
	store := newStubConfigRepo()
	store.saveErr = errors.New("read-only filesystem")

	_, err := patchWith(t, store, `{"vault":{"ttl":"48h"}}`)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "read-only filesystem")
}

func TestConfigUsecase_Patch_PropagatesLoadError(t *testing.T) {
	store := newStubConfigRepo()
	store.loadErr = errors.New("disk on fire")

	_, err := patchWith(t, store, `{"vault":{"ttl":"48h"}}`)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "disk on fire")
}

func TestConfigUsecase_Patch_IgnoresUntouchedInvalidSetting(t *testing.T) {
	store := newStubConfigRepo()
	store.configured.Logger.Level = "banana"

	result, err := patchWith(t, store, `{"vault":{"ttl":"48h"}}`)

	require.NoError(t, err)
	assert.Equal(t, []string{"vault.ttl"}, result.Applied)
	assert.Empty(t, result.Rejected)
}

// Every setting in the configuration must be reachable through the API without
// per-setting code, which is the whole point of addressing them by dotted key.
func TestConfigUsecase_Patch_ReachesEverySetting(t *testing.T) {
	for _, key := range coreconfig.Keys() {
		t.Run(key, func(t *testing.T) {
			store := newStubConfigRepo()

			result, err := patchWith(t, store, nestedBody(key, "null"))

			require.NoError(t, err)
			assert.Equal(t, []string{key}, result.Applied)
			assert.Empty(t, result.Rejected)
		})
	}
}

// nestedBody builds the sparse document that addresses one dotted key.
func nestedBody(key, value string) string {
	segments := strings.Split(key, ".")

	body := value
	for i := len(segments) - 1; i >= 0; i-- {
		body = `{"` + segments[i] + `":` + body + `}`
	}

	return body
}

func TestFlatten_EmptyBodyYieldsNoSettings(t *testing.T) {
	settings, err := flatten(nil)

	require.NoError(t, err)
	assert.Empty(t, settings)
}

func TestFlatten_SkipsLeadingWhitespace(t *testing.T) {
	settings, err := flatten(json.RawMessage("  \n\t{\"vault\": {\"ttl\": \"48h\"}}"))

	require.NoError(t, err)
	assert.Len(t, settings, 1)
	assert.Contains(t, settings, "vault.ttl")
}

func TestFlatten_EmptyValueIsNotAnObject(t *testing.T) {
	_, err := flatten(json.RawMessage(`{"vault":{"ttl":""}}`))

	require.NoError(t, err)
}

func TestFlatten_NestedNonObjectIsALeaf(t *testing.T) {
	settings, err := flatten(json.RawMessage(`{"arrows":{"auto_retry":{"retries":5}}}`))

	require.NoError(t, err)
	assert.Contains(t, settings, "arrows.auto_retry.retries")
}

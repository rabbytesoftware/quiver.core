package usecases

import (
	"context"
	"fmt"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/core/config"
)

const (
	keyPortStart = "netbridge.ephemeral_port_start"
	keyPortEnd   = "netbridge.ephemeral_port_end"
	keyHost      = "api.host"
)

// ConfigUsecase reads and edits the daemon configuration. Every change takes
// effect on the next daemon start; nothing here mutates the running process.
type ConfigUsecase interface {
	// Get returns the running, configured and default configurations together
	// with the fields whose change is still waiting on a restart.
	Get(ctx context.Context) (ConfigView, error)

	// Patch persists the fields the patch sets and reports the ones it
	// refused. It returns ErrInvalidConfig when nothing at all could be
	// applied.
	Patch(ctx context.Context, patch ConfigPatch) (PatchResult, error)
}

type configUsecase struct {
	store ConfigStore
}

// NewConfigUsecase returns a ConfigUsecase backed by the given store.
func NewConfigUsecase(
	store ConfigStore,
) ConfigUsecase {
	return &configUsecase{store: store}
}

func (u *configUsecase) Get(
	_ context.Context,
) (ConfigView, error) {
	configured, err := u.store.Configured()
	if err != nil {
		return ConfigView{}, fmt.Errorf("get config: read configured: %w", err)
	}

	running := u.store.Running()

	return ConfigView{
		Running:         running,
		Configured:      configured,
		Defaults:        u.store.Defaults(),
		RestartRequired: pendingKeys(running, configured),
	}, nil
}

func (u *configUsecase) Patch(
	_ context.Context,
	patch ConfigPatch,
) (PatchResult, error) {
	configured, err := u.store.Configured()
	if err != nil {
		return PatchResult{}, fmt.Errorf("patch config: read configured: %w", err)
	}

	next := configured
	touched := applyPatch(&next, u.store.Defaults(), patch)

	if len(touched) == 0 {
		return PatchResult{}, nil
	}

	applied, rejected := u.settle(&next, configured, touched)

	if len(applied) == 0 {
		return PatchResult{Rejected: rejected}, fmt.Errorf(
			"patch config: %w: %s: %s",
			apperrors.ErrInvalidConfig, rejected[0].Key, rejected[0].Message,
		)
	}

	if err := u.store.Save(next); err != nil {
		return PatchResult{}, fmt.Errorf("patch config: save: %w", err)
	}

	return PatchResult{Applied: applied, Rejected: rejected}, nil
}

func (u *configUsecase) settle(
	next *config.ConfigData,
	configured config.ConfigData,
	touched []string,
) ([]string, []config.FieldError) {
	index := make(map[string]bool, len(touched))
	for _, key := range touched {
		index[key] = true
	}

	var rejected []config.FieldError

	for _, fe := range u.store.Validate(*next) {
		blame := blameKey(fe.Key, index)
		if blame == "" {
			continue
		}

		config.RestoreField(next, configured, blame)
		rejected = append(rejected, config.FieldError{Key: blame, Message: fe.Message})
		delete(index, blame)
	}

	applied := make([]string, 0, len(index))
	for _, key := range touched {
		if index[key] {
			applied = append(applied, key)
		}
	}

	return applied, rejected
}

// blameKey attributes a validation failure to a field the caller actually
// sent. The port range is the one rule spanning two fields, so raising the
// start above an untouched end must be reported against the start.
func blameKey(
	key string,
	touched map[string]bool,
) string {
	if touched[key] {
		return key
	}

	if key == keyPortEnd && touched[keyPortStart] {
		return keyPortStart
	}

	if key == keyPortStart && touched[keyPortEnd] {
		return keyPortEnd
	}

	return ""
}

func applyPatch(
	next *config.ConfigData,
	def config.ConfigData,
	patch ConfigPatch,
) []string {
	var touched []string

	applyLeaf(&next.Netbridge.Enabled, patch.Netbridge.Enabled,
		def.Netbridge.Enabled, "netbridge.enabled", &touched)
	applyLeaf(&next.Netbridge.EphemeralPortStart, patch.Netbridge.EphemeralPortStart,
		def.Netbridge.EphemeralPortStart, keyPortStart, &touched)
	applyLeaf(&next.Netbridge.EphemeralPortEnd, patch.Netbridge.EphemeralPortEnd,
		def.Netbridge.EphemeralPortEnd, keyPortEnd, &touched)
	applyLeaf(&next.API.Host, patch.API.Host, def.API.Host, keyHost, &touched)
	applyLeaf(&next.Logger.Enabled, patch.Logger.Enabled,
		def.Logger.Enabled, "logger.enabled", &touched)
	applyLeaf(&next.Logger.Level, patch.Logger.Level,
		def.Logger.Level, "logger.level", &touched)
	applyLeaf(&next.Manifold.FetchTimeout, patch.Manifold.FetchTimeout,
		def.Manifold.FetchTimeout, "manifold.fetch_timeout", &touched)
	applyVaultPatch(next, def, patch, &touched)
	applyLeaf(&next.Arrows.AutoRetry.Enabled, patch.Arrows.AutoRetry.Enabled,
		def.Arrows.AutoRetry.Enabled, "arrows.auto_retry.enabled", &touched)
	applyLeaf(&next.Arrows.AutoRetry.Retries, patch.Arrows.AutoRetry.Retries,
		def.Arrows.AutoRetry.Retries, "arrows.auto_retry.retries", &touched)
	applySearchPatch(next, def, patch, &touched)

	return touched
}

func applyVaultPatch(
	next *config.ConfigData,
	def config.ConfigData,
	patch ConfigPatch,
	touched *[]string,
) {
	applyLeaf(&next.Vault.SweepInterval, patch.Vault.SweepInterval,
		def.Vault.SweepInterval, "vault.sweep_interval", touched)
	applyLeaf(&next.Vault.TTL, patch.Vault.TTL, def.Vault.TTL, "vault.ttl", touched)
	applyLeaf(&next.Vault.IndexTTL, patch.Vault.IndexTTL,
		def.Vault.IndexTTL, "vault.index_ttl", touched)
}

func applySearchPatch(
	next *config.ConfigData,
	def config.ConfigData,
	patch ConfigPatch,
	touched *[]string,
) {
	applyLeaf(&next.Search.PerProviderLimit, patch.Search.PerProviderLimit,
		def.Search.PerProviderLimit, "search.per_provider_limit", touched)
	applyLeaf(&next.Search.FetchConcurrency, patch.Search.FetchConcurrency,
		def.Search.FetchConcurrency, "search.fetch_concurrency", touched)
	applyLeaf(&next.Search.ProviderTimeout, patch.Search.ProviderTimeout,
		def.Search.ProviderTimeout, "search.provider_timeout", touched)
}

func applyLeaf[T Leaf](
	target *T,
	opt Optional[T],
	def T,
	key string,
	touched *[]string,
) {
	if !opt.IsSet() {
		return
	}

	if opt.IsReset() {
		*target = def
	} else {
		*target = opt.Value()
	}

	*touched = append(*touched, key)
}

// pendingKeys lists the fields whose configured value differs from the one the
// process is running with. api.host is excluded: the --host flag can override
// it at start, so the running value is not knowable from configuration alone.
func pendingKeys(
	running config.ConfigData,
	configured config.ConfigData,
) []string {
	var keys []string

	keys = appendIfNot(keys, running.Netbridge.Enabled == configured.Netbridge.Enabled, "netbridge.enabled")
	keys = appendIfNot(keys, running.Netbridge.EphemeralPortStart == configured.Netbridge.EphemeralPortStart, keyPortStart)
	keys = appendIfNot(keys, running.Netbridge.EphemeralPortEnd == configured.Netbridge.EphemeralPortEnd, keyPortEnd)
	keys = appendIfNot(keys, running.Logger.Enabled == configured.Logger.Enabled, "logger.enabled")
	keys = appendIfNot(keys, running.Logger.Level == configured.Logger.Level, "logger.level")
	keys = appendIfNot(keys, running.Manifold.FetchTimeout == configured.Manifold.FetchTimeout, "manifold.fetch_timeout")
	keys = appendIfNot(keys, running.Vault.SweepInterval == configured.Vault.SweepInterval, "vault.sweep_interval")
	keys = appendIfNot(keys, running.Vault.TTL == configured.Vault.TTL, "vault.ttl")
	keys = appendIfNot(keys, running.Vault.IndexTTL == configured.Vault.IndexTTL, "vault.index_ttl")
	keys = appendIfNot(keys, running.Arrows.AutoRetry.Enabled == configured.Arrows.AutoRetry.Enabled, "arrows.auto_retry.enabled")
	keys = appendIfNot(keys, running.Arrows.AutoRetry.Retries == configured.Arrows.AutoRetry.Retries, "arrows.auto_retry.retries")
	keys = appendIfNot(keys, running.Search.PerProviderLimit == configured.Search.PerProviderLimit, "search.per_provider_limit")
	keys = appendIfNot(keys, running.Search.FetchConcurrency == configured.Search.FetchConcurrency, "search.fetch_concurrency")
	keys = appendIfNot(keys, running.Search.ProviderTimeout == configured.Search.ProviderTimeout, "search.provider_timeout")

	return keys
}

func appendIfNot(
	keys []string,
	same bool,
	key string,
) []string {
	if same {
		return keys
	}

	return append(keys, key)
}

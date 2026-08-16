package usecases

import (
	"context"
	"encoding/json"
	"fmt"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	repoconfig "github.com/rabbytesoftware/quiver.core/internal/app/repositories/config"
)

// Leaf constrains the value types a configuration field can hold.
type Leaf interface{ bool | int | string }

// Optional distinguishes a JSON field that was absent from one explicitly set
// to null and from one carrying a value. A configuration patch needs all three:
// absent means leave alone, null means restore the default, and a value means
// set it.
type Optional[T Leaf] struct {
	set   bool
	reset bool
	value T
}

// IsSet reports whether the field was present in the request body at all.
func (o Optional[T]) IsSet() bool {
	return o.set
}

// IsReset reports whether the field was present and explicitly null.
func (o Optional[T]) IsReset() bool {
	return o.reset
}

// Value returns the decoded value. It is meaningful only when IsSet reports
// true and IsReset reports false.
func (o Optional[T]) Value() T {
	return o.value
}

// UnmarshalJSON records that the field was present, then decodes it. It is
// never called for an absent field, which is what makes absent and null
// distinguishable.
func (o *Optional[T]) UnmarshalJSON(
	data []byte,
) error {
	o.set = true

	if string(data) == "null" {
		o.reset = true
		return nil
	}

	return json.Unmarshal(data, &o.value)
}

// ConfigPatch is a sparse configuration change. Every field is optional: an
// absent field is left alone, a null field is restored to its default, and a
// field carrying a value is set to it.
//
// Each field is documented as its underlying scalar rather than as the
// Optional wrapper, because that is what goes on the wire.
type ConfigPatch struct {
	Netbridge NetbridgePatch `json:"netbridge"`
	API       APIPatch       `json:"api"`
	Logger    LoggerPatch    `json:"logger"`
	Manifold  ManifoldPatch  `json:"manifold"`
	Vault     VaultPatch     `json:"vault"`
	Arrows    ArrowsPatch    `json:"arrows"`
	Search    SearchPatch    `json:"search"`
}

// NetbridgePatch is the netbridge section of a ConfigPatch.
type NetbridgePatch struct {
	Enabled            Optional[bool] `json:"enabled" swaggertype:"boolean" extensions:"x-nullable"`
	EphemeralPortStart Optional[int]  `json:"ephemeral_port_start" swaggertype:"integer" extensions:"x-nullable"`
	EphemeralPortEnd   Optional[int]  `json:"ephemeral_port_end" swaggertype:"integer" extensions:"x-nullable"`
}

// APIPatch is the api section of a ConfigPatch.
type APIPatch struct {
	Host Optional[string] `json:"host" swaggertype:"string" extensions:"x-nullable"`
}

// LoggerPatch is the logger section of a ConfigPatch.
type LoggerPatch struct {
	Enabled Optional[bool]   `json:"enabled" swaggertype:"boolean" extensions:"x-nullable"`
	Level   Optional[string] `json:"level" swaggertype:"string" extensions:"x-nullable"`
}

// ManifoldPatch is the manifold section of a ConfigPatch.
type ManifoldPatch struct {
	FetchTimeout Optional[string] `json:"fetch_timeout" swaggertype:"string" extensions:"x-nullable"`
}

// VaultPatch is the vault section of a ConfigPatch.
type VaultPatch struct {
	SweepInterval Optional[string] `json:"sweep_interval" swaggertype:"string" extensions:"x-nullable"`
	TTL           Optional[string] `json:"ttl" swaggertype:"string" extensions:"x-nullable"`
	IndexTTL      Optional[string] `json:"index_ttl" swaggertype:"string" extensions:"x-nullable"`
}

// ArrowsPatch is the arrows section of a ConfigPatch.
type ArrowsPatch struct {
	AutoRetry AutoRetryPatch `json:"auto_retry"`
}

// AutoRetryPatch is the arrows.auto_retry subsection of a ConfigPatch.
type AutoRetryPatch struct {
	Enabled Optional[bool] `json:"enabled" swaggertype:"boolean" extensions:"x-nullable"`
	Retries Optional[int]  `json:"retries" swaggertype:"integer" extensions:"x-nullable"`
}

// SearchPatch is the search section of a ConfigPatch.
type SearchPatch struct {
	PerProviderLimit Optional[int]    `json:"per_provider_limit" swaggertype:"integer" extensions:"x-nullable"`
	FetchConcurrency Optional[int]    `json:"fetch_concurrency" swaggertype:"integer" extensions:"x-nullable"`
	ProviderTimeout  Optional[string] `json:"provider_timeout" swaggertype:"string" extensions:"x-nullable"`
}

// ConfigView is the daemon configuration seen three ways at once: what the
// process is running with, what the next start will use, and what ships in the
// binary. A client needs all three to show a current value, offer a reset, and
// say honestly whether a change has taken effect.
type ConfigView struct {
	Running         repoconfig.Data
	Configured      repoconfig.Data
	Defaults        repoconfig.Data
	RestartRequired []string
}

// PatchResult reports which fields a patch persisted and which it refused.
type PatchResult struct {
	Applied  []string
	Rejected []repoconfig.FieldError
}

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
	repo repoconfig.Config
}

// NewConfigUsecase returns a ConfigUsecase backed by the given repository.
func NewConfigUsecase(
	repo repoconfig.Config,
) ConfigUsecase {
	return &configUsecase{repo: repo}
}

func (u *configUsecase) Get(
	_ context.Context,
) (ConfigView, error) {
	configured, err := u.repo.Configured()
	if err != nil {
		return ConfigView{}, fmt.Errorf("get config: read configured: %w", err)
	}

	running := u.repo.Running()

	return ConfigView{
		Running:         running,
		Configured:      configured,
		Defaults:        u.repo.Defaults(),
		RestartRequired: pendingKeys(running, configured),
	}, nil
}

func (u *configUsecase) Patch(
	_ context.Context,
	patch ConfigPatch,
) (PatchResult, error) {
	configured, err := u.repo.Configured()
	if err != nil {
		return PatchResult{}, fmt.Errorf("patch config: read configured: %w", err)
	}

	next := configured
	touched := applyPatch(&next, u.repo.Defaults(), patch)

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

	if err := u.repo.Save(next); err != nil {
		return PatchResult{}, fmt.Errorf("patch config: save: %w", err)
	}

	return PatchResult{Applied: applied, Rejected: rejected}, nil
}

func (u *configUsecase) settle(
	next *repoconfig.Data,
	configured repoconfig.Data,
	touched []string,
) ([]string, []repoconfig.FieldError) {
	index := make(map[string]bool, len(touched))
	for _, key := range touched {
		index[key] = true
	}

	var rejected []repoconfig.FieldError

	for _, fe := range u.repo.Validate(*next) {
		blame := blameKey(fe.Key, index)
		if blame == "" {
			continue
		}

		repoconfig.RestoreField(next, configured, blame)
		rejected = append(rejected, repoconfig.FieldError{Key: blame, Message: fe.Message})
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
	next *repoconfig.Data,
	def repoconfig.Data,
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
	next *repoconfig.Data,
	def repoconfig.Data,
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
	next *repoconfig.Data,
	def repoconfig.Data,
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
	running repoconfig.Data,
	configured repoconfig.Data,
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

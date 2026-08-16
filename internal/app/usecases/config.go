package usecases

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

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

		restoreField(next, configured, blame)
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

// fieldOp is one configuration leaf, with its value type erased from the
// caller but not from the implementation: every method below is closed over a
// concrete T, so nothing is ever asserted back out.
type fieldOp interface {
	Key() string
	ApplyPatch(next *repoconfig.Data, def repoconfig.Data, patch ConfigPatch) bool
	Differs(a, b repoconfig.Data) bool
	Restore(dst *repoconfig.Data, src repoconfig.Data)
}

type field[T Leaf] struct {
	key  string
	get  func(repoconfig.Data) T
	set  func(*repoconfig.Data, T)
	from func(ConfigPatch) Optional[T]
}

func (f field[T]) Key() string {
	return f.key
}

func (f field[T]) Differs(
	a repoconfig.Data,
	b repoconfig.Data,
) bool {
	return f.get(a) != f.get(b)
}

func (f field[T]) Restore(
	dst *repoconfig.Data,
	src repoconfig.Data,
) {
	f.set(dst, f.get(src))
}

func (f field[T]) ApplyPatch(
	next *repoconfig.Data,
	def repoconfig.Data,
	patch ConfigPatch,
) bool {
	opt := f.from(patch)
	if !opt.IsSet() {
		return false
	}

	if opt.IsReset() {
		f.set(next, f.get(def))
		return true
	}

	f.set(next, opt.Value())

	return true
}

// configFields is the single enumeration of the configuration surface.
// Adding a setting means adding one entry here; patching, restoring and the
// restart-pending diff all derive from it.
func configFields() []fieldOp {
	return slices.Concat(
		netbridgeFields(),
		apiFields(),
		loggerFields(),
		manifoldFields(),
		vaultFields(),
		arrowsFields(),
		searchFields(),
	)
}

func netbridgeFields() []fieldOp {
	return []fieldOp{
		field[bool]{
			key:  "netbridge.enabled",
			get:  func(d repoconfig.Data) bool { return d.Netbridge.Enabled },
			set:  func(d *repoconfig.Data, v bool) { d.Netbridge.Enabled = v },
			from: func(p ConfigPatch) Optional[bool] { return p.Netbridge.Enabled },
		},
		field[int]{
			key:  keyPortStart,
			get:  func(d repoconfig.Data) int { return d.Netbridge.EphemeralPortStart },
			set:  func(d *repoconfig.Data, v int) { d.Netbridge.EphemeralPortStart = v },
			from: func(p ConfigPatch) Optional[int] { return p.Netbridge.EphemeralPortStart },
		},
		field[int]{
			key:  keyPortEnd,
			get:  func(d repoconfig.Data) int { return d.Netbridge.EphemeralPortEnd },
			set:  func(d *repoconfig.Data, v int) { d.Netbridge.EphemeralPortEnd = v },
			from: func(p ConfigPatch) Optional[int] { return p.Netbridge.EphemeralPortEnd },
		},
	}
}

func apiFields() []fieldOp {
	return []fieldOp{
		field[string]{
			key:  keyHost,
			get:  func(d repoconfig.Data) string { return d.API.Host },
			set:  func(d *repoconfig.Data, v string) { d.API.Host = v },
			from: func(p ConfigPatch) Optional[string] { return p.API.Host },
		},
	}
}

func loggerFields() []fieldOp {
	return []fieldOp{
		field[bool]{
			key:  "logger.enabled",
			get:  func(d repoconfig.Data) bool { return d.Logger.Enabled },
			set:  func(d *repoconfig.Data, v bool) { d.Logger.Enabled = v },
			from: func(p ConfigPatch) Optional[bool] { return p.Logger.Enabled },
		},
		field[string]{
			key:  "logger.level",
			get:  func(d repoconfig.Data) string { return d.Logger.Level },
			set:  func(d *repoconfig.Data, v string) { d.Logger.Level = v },
			from: func(p ConfigPatch) Optional[string] { return p.Logger.Level },
		},
	}
}

func manifoldFields() []fieldOp {
	return []fieldOp{
		field[string]{
			key:  "manifold.fetch_timeout",
			get:  func(d repoconfig.Data) string { return d.Manifold.FetchTimeout },
			set:  func(d *repoconfig.Data, v string) { d.Manifold.FetchTimeout = v },
			from: func(p ConfigPatch) Optional[string] { return p.Manifold.FetchTimeout },
		},
	}
}

func vaultFields() []fieldOp {
	return []fieldOp{
		field[string]{
			key:  "vault.sweep_interval",
			get:  func(d repoconfig.Data) string { return d.Vault.SweepInterval },
			set:  func(d *repoconfig.Data, v string) { d.Vault.SweepInterval = v },
			from: func(p ConfigPatch) Optional[string] { return p.Vault.SweepInterval },
		},
		field[string]{
			key:  "vault.ttl",
			get:  func(d repoconfig.Data) string { return d.Vault.TTL },
			set:  func(d *repoconfig.Data, v string) { d.Vault.TTL = v },
			from: func(p ConfigPatch) Optional[string] { return p.Vault.TTL },
		},
		field[string]{
			key:  "vault.index_ttl",
			get:  func(d repoconfig.Data) string { return d.Vault.IndexTTL },
			set:  func(d *repoconfig.Data, v string) { d.Vault.IndexTTL = v },
			from: func(p ConfigPatch) Optional[string] { return p.Vault.IndexTTL },
		},
	}
}

func arrowsFields() []fieldOp {
	return []fieldOp{
		field[bool]{
			key:  "arrows.auto_retry.enabled",
			get:  func(d repoconfig.Data) bool { return d.Arrows.AutoRetry.Enabled },
			set:  func(d *repoconfig.Data, v bool) { d.Arrows.AutoRetry.Enabled = v },
			from: func(p ConfigPatch) Optional[bool] { return p.Arrows.AutoRetry.Enabled },
		},
		field[int]{
			key:  "arrows.auto_retry.retries",
			get:  func(d repoconfig.Data) int { return d.Arrows.AutoRetry.Retries },
			set:  func(d *repoconfig.Data, v int) { d.Arrows.AutoRetry.Retries = v },
			from: func(p ConfigPatch) Optional[int] { return p.Arrows.AutoRetry.Retries },
		},
	}
}

func searchFields() []fieldOp {
	return []fieldOp{
		field[int]{
			key:  "search.per_provider_limit",
			get:  func(d repoconfig.Data) int { return d.Search.PerProviderLimit },
			set:  func(d *repoconfig.Data, v int) { d.Search.PerProviderLimit = v },
			from: func(p ConfigPatch) Optional[int] { return p.Search.PerProviderLimit },
		},
		field[int]{
			key:  "search.fetch_concurrency",
			get:  func(d repoconfig.Data) int { return d.Search.FetchConcurrency },
			set:  func(d *repoconfig.Data, v int) { d.Search.FetchConcurrency = v },
			from: func(p ConfigPatch) Optional[int] { return p.Search.FetchConcurrency },
		},
		field[string]{
			key:  "search.provider_timeout",
			get:  func(d repoconfig.Data) string { return d.Search.ProviderTimeout },
			set:  func(d *repoconfig.Data, v string) { d.Search.ProviderTimeout = v },
			from: func(p ConfigPatch) Optional[string] { return p.Search.ProviderTimeout },
		},
	}
}

func applyPatch(
	next *repoconfig.Data,
	def repoconfig.Data,
	patch ConfigPatch,
) []string {
	var touched []string

	for _, f := range configFields() {
		if f.ApplyPatch(next, def, patch) {
			touched = append(touched, f.Key())
		}
	}

	return touched
}

func restoreField(
	next *repoconfig.Data,
	src repoconfig.Data,
	key string,
) {
	for _, f := range configFields() {
		if f.Key() == key {
			f.Restore(next, src)
			return
		}
	}
}

// pendingKeys lists the fields whose configured value differs from the one the
// process is running with. api.host is excluded: the --host flag can override
// it at start, so the running value is not knowable from configuration alone.
func pendingKeys(
	running repoconfig.Data,
	configured repoconfig.Data,
) []string {
	var keys []string

	for _, f := range configFields() {
		if f.Key() == keyHost {
			continue
		}

		if f.Differs(running, configured) {
			keys = append(keys, f.Key())
		}
	}

	return keys
}

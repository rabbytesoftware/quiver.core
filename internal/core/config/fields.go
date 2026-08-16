package config

import (
	"fmt"
	"slices"
)

// Leaf constrains the value types a configuration field can hold.
type Leaf interface{ bool | int | string }

// Field is one configuration leaf. Its value type is erased from the caller
// but not from the implementation: every method is closed over a concrete
// type, so no value is ever asserted back out.
type Field interface {
	// Key returns the dotted path the configuration API addresses this field by.
	Key() string

	// Differs reports whether two configurations disagree on this field.
	Differs(a, b ConfigData) bool

	// Restore copies this field, and only this field, from src into dst.
	Restore(dst *ConfigData, src ConfigData)

	// Check reports why the daemon cannot use this field's value, or nil.
	Check(data ConfigData) *FieldError
}

type field[T Leaf] struct {
	key     string
	get     func(ConfigData) T
	set     func(*ConfigData, T)
	valid   func(T) bool
	message func(T) string
}

func (f field[T]) Key() string {
	return f.key
}

func (f field[T]) Differs(
	a, b ConfigData,
) bool {
	return f.get(a) != f.get(b)
}

func (f field[T]) Restore(
	dst *ConfigData,
	src ConfigData,
) {
	f.set(dst, f.get(src))
}

func (f field[T]) Check(
	data ConfigData,
) *FieldError {
	value := f.get(data)
	if f.valid == nil || f.valid(value) {
		return nil
	}

	return &FieldError{Key: f.key, Message: f.message(value)}
}

// Fields is the single enumeration of the configuration surface. Adding a
// setting means adding one entry here; validation, per-field fallback and
// single-field restore all derive from it.
func Fields() []Field {
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

// Keys lists every configuration field by dotted path, in Fields order.
func Keys() []string {
	fields := Fields()

	keys := make([]string, 0, len(fields))
	for _, f := range fields {
		keys = append(keys, f.Key())
	}

	return keys
}

func netbridgeFields() []Field {
	return []Field{
		field[bool]{
			key: "netbridge.enabled",
			get: func(d ConfigData) bool { return d.Netbridge.Enabled },
			set: func(d *ConfigData, v bool) { d.Netbridge.Enabled = v },
		},
		field[int]{
			key:     keyPortStart,
			get:     func(d ConfigData) int { return d.Netbridge.EphemeralPortStart },
			set:     func(d *ConfigData, v int) { d.Netbridge.EphemeralPortStart = v },
			valid:   validPort,
			message: portMessage,
		},
		field[int]{
			key:     keyPortEnd,
			get:     func(d ConfigData) int { return d.Netbridge.EphemeralPortEnd },
			set:     func(d *ConfigData, v int) { d.Netbridge.EphemeralPortEnd = v },
			valid:   validPort,
			message: portMessage,
		},
	}
}

func apiFields() []Field {
	return []Field{
		field[string]{
			key:     "api.host",
			get:     func(d ConfigData) string { return d.API.Host },
			set:     func(d *ConfigData, v string) { d.API.Host = v },
			valid:   validHost,
			message: hostMessage,
		},
	}
}

func loggerFields() []Field {
	return []Field{
		field[bool]{
			key: "logger.enabled",
			get: func(d ConfigData) bool { return d.Logger.Enabled },
			set: func(d *ConfigData, v bool) { d.Logger.Enabled = v },
		},
		field[string]{
			key:     "logger.level",
			get:     func(d ConfigData) string { return d.Logger.Level },
			set:     func(d *ConfigData, v string) { d.Logger.Level = v },
			valid:   validLogLevel,
			message: levelMessage,
		},
	}
}

func manifoldFields() []Field {
	return []Field{
		field[string]{
			key:     "manifold.fetch_timeout",
			get:     func(d ConfigData) string { return d.Manifold.FetchTimeout },
			set:     func(d *ConfigData, v string) { d.Manifold.FetchTimeout = v },
			valid:   validDuration,
			message: durationMessage,
		},
	}
}

func vaultFields() []Field {
	return []Field{
		field[string]{
			key:     "vault.sweep_interval",
			get:     func(d ConfigData) string { return d.Vault.SweepInterval },
			set:     func(d *ConfigData, v string) { d.Vault.SweepInterval = v },
			valid:   validDuration,
			message: durationMessage,
		},
		field[string]{
			key:     "vault.ttl",
			get:     func(d ConfigData) string { return d.Vault.TTL },
			set:     func(d *ConfigData, v string) { d.Vault.TTL = v },
			valid:   validDuration,
			message: durationMessage,
		},
		field[string]{
			key:     "vault.index_ttl",
			get:     func(d ConfigData) string { return d.Vault.IndexTTL },
			set:     func(d *ConfigData, v string) { d.Vault.IndexTTL = v },
			valid:   validDuration,
			message: durationMessage,
		},
	}
}

func arrowsFields() []Field {
	return []Field{
		field[bool]{
			key: "arrows.auto_retry.enabled",
			get: func(d ConfigData) bool { return d.Arrows.AutoRetry.Enabled },
			set: func(d *ConfigData, v bool) { d.Arrows.AutoRetry.Enabled = v },
		},
		field[int]{
			key:     "arrows.auto_retry.retries",
			get:     func(d ConfigData) int { return d.Arrows.AutoRetry.Retries },
			set:     func(d *ConfigData, v int) { d.Arrows.AutoRetry.Retries = v },
			valid:   notNegative,
			message: retriesMessage,
		},
	}
}

func searchFields() []Field {
	return []Field{
		field[int]{
			key:     "search.per_provider_limit",
			get:     func(d ConfigData) int { return d.Search.PerProviderLimit },
			set:     func(d *ConfigData, v int) { d.Search.PerProviderLimit = v },
			valid:   atLeastOne,
			message: atLeastOneMessage,
		},
		field[int]{
			key:     "search.fetch_concurrency",
			get:     func(d ConfigData) int { return d.Search.FetchConcurrency },
			set:     func(d *ConfigData, v int) { d.Search.FetchConcurrency = v },
			valid:   atLeastOne,
			message: atLeastOneMessage,
		},
		field[string]{
			key:     "search.provider_timeout",
			get:     func(d ConfigData) string { return d.Search.ProviderTimeout },
			set:     func(d *ConfigData, v string) { d.Search.ProviderTimeout = v },
			valid:   validDuration,
			message: durationMessage,
		},
	}
}

func notNegative(v int) bool {
	return v >= 0
}

func atLeastOne(v int) bool {
	return v >= 1
}

func portMessage(got int) string {
	return fmt.Sprintf("must be a port between %d and %d, got %d", minPort, maxPort, got)
}

func durationMessage(got string) string {
	return fmt.Sprintf("must be a positive duration such as 30s or 5m, got %q", got)
}

func atLeastOneMessage(got int) string {
	return fmt.Sprintf("must be at least 1, got %d", got)
}

func retriesMessage(got int) string {
	return fmt.Sprintf("must be zero or greater, got %d", got)
}

func hostMessage(got string) string {
	return fmt.Sprintf(
		"must be a unix:// or tcp://host:port URI, got %q; recover a running daemon with --host",
		got,
	)
}

func levelMessage(got string) string {
	return fmt.Sprintf(
		"must be one of debug, trace, info, warn, warning, error, fatal, panic, got %q",
		got,
	)
}

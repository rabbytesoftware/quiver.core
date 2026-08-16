package config

import "fmt"

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

// field addresses its leaf through a single pointer accessor: reading,
// writing, comparing and restoring all derive from it, and the accessor is an
// ordinary field reference the compiler checks.
type field[T Leaf] struct {
	key   string
	ptr   func(*ConfigData) *T
	check func(T) string
}

func (f field[T]) Key() string {
	return f.key
}

func (f field[T]) Differs(
	a, b ConfigData,
) bool {
	return *f.ptr(&a) != *f.ptr(&b)
}

func (f field[T]) Restore(
	dst *ConfigData,
	src ConfigData,
) {
	*f.ptr(dst) = *f.ptr(&src)
}

func (f field[T]) Check(
	data ConfigData,
) *FieldError {
	if f.check == nil {
		return nil
	}

	message := f.check(*f.ptr(&data))
	if message == "" {
		return nil
	}

	return &FieldError{Key: f.key, Message: message}
}

// Fields is the single enumeration of the configuration surface. Adding a
// setting means adding one line here; validation, per-field fallback and
// single-field restore all derive from it.
func Fields() []Field {
	return []Field{
		boolField("netbridge.enabled", func(c *ConfigData) *bool { return &c.Netbridge.Enabled }),
		intField(keyPortStart, func(c *ConfigData) *int { return &c.Netbridge.EphemeralPortStart }, portCheck),
		intField(keyPortEnd, func(c *ConfigData) *int { return &c.Netbridge.EphemeralPortEnd }, portCheck),
		strField("api.host", func(c *ConfigData) *string { return &c.API.Host }, hostCheck),
		boolField("logger.enabled", func(c *ConfigData) *bool { return &c.Logger.Enabled }),
		strField("logger.level", func(c *ConfigData) *string { return &c.Logger.Level }, levelCheck),
		strField("manifold.fetch_timeout", func(c *ConfigData) *string { return &c.Manifold.FetchTimeout }, durationCheck),
		strField("vault.sweep_interval", func(c *ConfigData) *string { return &c.Vault.SweepInterval }, durationCheck),
		strField("vault.ttl", func(c *ConfigData) *string { return &c.Vault.TTL }, durationCheck),
		strField("vault.index_ttl", func(c *ConfigData) *string { return &c.Vault.IndexTTL }, durationCheck),
		boolField("arrows.auto_retry.enabled", func(c *ConfigData) *bool { return &c.Arrows.AutoRetry.Enabled }),
		intField("arrows.auto_retry.retries", func(c *ConfigData) *int { return &c.Arrows.AutoRetry.Retries }, retriesCheck),
		intField("search.per_provider_limit", func(c *ConfigData) *int { return &c.Search.PerProviderLimit }, atLeastOneCheck),
		intField("search.fetch_concurrency", func(c *ConfigData) *int { return &c.Search.FetchConcurrency }, atLeastOneCheck),
		strField("search.provider_timeout", func(c *ConfigData) *string { return &c.Search.ProviderTimeout }, durationCheck),
	}
}

// boolField takes no check: no boolean configuration value is invalid.
func boolField(
	key string,
	ptr func(*ConfigData) *bool,
) Field {
	return field[bool]{key: key, ptr: ptr}
}

func intField(
	key string,
	ptr func(*ConfigData) *int,
	check func(int) string,
) Field {
	return field[int]{key: key, ptr: ptr, check: check}
}

func strField(
	key string,
	ptr func(*ConfigData) *string,
	check func(string) string,
) Field {
	return field[string]{key: key, ptr: ptr, check: check}
}

func portCheck(got int) string {
	if validPort(got) {
		return ""
	}

	return fmt.Sprintf("must be a port between %d and %d, got %d", minPort, maxPort, got)
}

func durationCheck(got string) string {
	if validDuration(got) {
		return ""
	}

	return fmt.Sprintf("must be a positive duration such as 30s or 5m, got %q", got)
}

func hostCheck(got string) string {
	if validHost(got) {
		return ""
	}

	return fmt.Sprintf(
		"must be a unix:// or tcp://host:port URI, got %q; recover a running daemon with --host",
		got,
	)
}

func levelCheck(got string) string {
	if validLogLevel(got) {
		return ""
	}

	return fmt.Sprintf(
		"must be one of debug, trace, info, warn, warning, error, fatal, panic, got %q",
		got,
	)
}

func retriesCheck(got int) string {
	if got >= 0 {
		return ""
	}

	return fmt.Sprintf("must be zero or greater, got %d", got)
}

func atLeastOneCheck(got int) string {
	if got >= 1 {
		return ""
	}

	return fmt.Sprintf("must be at least 1, got %d", got)
}

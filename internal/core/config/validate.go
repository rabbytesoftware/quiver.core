package config

import "fmt"

// Defaults returns the configuration compiled into the binary, before any
// user overlay is applied.
func Defaults() ConfigData {
	return getDefaultConfig().Config
}

// Validate reports every configuration field whose value the daemon cannot
// use, keyed by the dotted path the configuration API addresses fields by.
// A valid configuration yields no errors.
func Validate(
	data ConfigData,
) []FieldError {
	var errs []FieldError

	errs = append(errs, validateNetbridge(data.Netbridge)...)
	errs = append(errs, validateAPI(data.API)...)
	errs = append(errs, validateLogger(data.Logger)...)
	errs = append(errs, validateManifold(data.Manifold)...)
	errs = append(errs, validateVault(data.Vault)...)
	errs = append(errs, validateArrows(data.Arrows)...)
	errs = append(errs, validateSearch(data.Search)...)

	return errs
}

// Sanitize replaces every invalid field with its compiled-in default and
// returns what it corrected. Valid fields are left untouched, so one bad
// value never discards the rest of a user's configuration.
func Sanitize(
	data *ConfigData,
) []FieldError {
	corrected := Validate(*data)
	def := Defaults()

	for _, fe := range corrected {
		restoreDefault(data, def, fe.Key)
	}

	return corrected
}

func restoreDefault(
	data *ConfigData,
	def ConfigData,
	key string,
) {
	switch key {
	case "netbridge.ephemeral_port_start":
		data.Netbridge.EphemeralPortStart = def.Netbridge.EphemeralPortStart
	case "netbridge.ephemeral_port_end":
		data.Netbridge.EphemeralPortEnd = def.Netbridge.EphemeralPortEnd
	case "api.host":
		data.API.Host = def.API.Host
	case "logger.level":
		data.Logger.Level = def.Logger.Level
	case "manifold.fetch_timeout":
		data.Manifold.FetchTimeout = def.Manifold.FetchTimeout
	case "vault.sweep_interval":
		data.Vault.SweepInterval = def.Vault.SweepInterval
	case "vault.ttl":
		data.Vault.TTL = def.Vault.TTL
	case "vault.index_ttl":
		data.Vault.IndexTTL = def.Vault.IndexTTL
	case "arrows.auto_retry.retries":
		data.Arrows.AutoRetry.Retries = def.Arrows.AutoRetry.Retries
	case "search.per_provider_limit":
		data.Search.PerProviderLimit = def.Search.PerProviderLimit
	case "search.fetch_concurrency":
		data.Search.FetchConcurrency = def.Search.FetchConcurrency
	case "search.provider_timeout":
		data.Search.ProviderTimeout = def.Search.ProviderTimeout
	}
}

func validateNetbridge(
	n Netbridge,
) []FieldError {
	var errs []FieldError

	if !validPort(n.EphemeralPortStart) {
		errs = append(errs, portError("netbridge.ephemeral_port_start", n.EphemeralPortStart))
	}

	if !validPort(n.EphemeralPortEnd) {
		errs = append(errs, portError("netbridge.ephemeral_port_end", n.EphemeralPortEnd))
		return errs
	}

	if validPort(n.EphemeralPortStart) && n.EphemeralPortStart > n.EphemeralPortEnd {
		errs = append(errs, FieldError{
			Key: "netbridge.ephemeral_port_end",
			Message: fmt.Sprintf(
				"must be greater than or equal to netbridge.ephemeral_port_start (%d), got %d",
				n.EphemeralPortStart, n.EphemeralPortEnd,
			),
		})
	}

	return errs
}

func validateAPI(
	a API,
) []FieldError {
	if validHost(a.Host) {
		return nil
	}

	return []FieldError{{
		Key: "api.host",
		Message: fmt.Sprintf(
			"must be a unix:// or tcp://host:port URI, got %q; recover a running daemon with --host",
			a.Host,
		),
	}}
}

func validateLogger(
	l Logger,
) []FieldError {
	if validLogLevel(l.Level) {
		return nil
	}

	return []FieldError{{
		Key: "logger.level",
		Message: fmt.Sprintf(
			"must be one of debug, trace, info, warn, warning, error, fatal, panic, got %q",
			l.Level,
		),
	}}
}

func validateManifold(
	m Manifold,
) []FieldError {
	if validDuration(m.FetchTimeout) {
		return nil
	}

	return []FieldError{durationError("manifold.fetch_timeout", m.FetchTimeout)}
}

func validateVault(
	v Vault,
) []FieldError {
	var errs []FieldError

	if !validDuration(v.SweepInterval) {
		errs = append(errs, durationError("vault.sweep_interval", v.SweepInterval))
	}

	if !validDuration(v.TTL) {
		errs = append(errs, durationError("vault.ttl", v.TTL))
	}

	if !validDuration(v.IndexTTL) {
		errs = append(errs, durationError("vault.index_ttl", v.IndexTTL))
	}

	return errs
}

func validateArrows(
	a Arrows,
) []FieldError {
	if a.AutoRetry.Retries >= 0 {
		return nil
	}

	return []FieldError{{
		Key:     "arrows.auto_retry.retries",
		Message: fmt.Sprintf("must be zero or greater, got %d", a.AutoRetry.Retries),
	}}
}

func validateSearch(
	s Search,
) []FieldError {
	var errs []FieldError

	if s.PerProviderLimit < 1 {
		errs = append(errs, atLeastOneError("search.per_provider_limit", s.PerProviderLimit))
	}

	if s.FetchConcurrency < 1 {
		errs = append(errs, atLeastOneError("search.fetch_concurrency", s.FetchConcurrency))
	}

	if !validDuration(s.ProviderTimeout) {
		errs = append(errs, durationError("search.provider_timeout", s.ProviderTimeout))
	}

	return errs
}

func portError(
	key string,
	got int,
) FieldError {
	return FieldError{
		Key:     key,
		Message: fmt.Sprintf("must be a port between %d and %d, got %d", minPort, maxPort, got),
	}
}

func durationError(
	key string,
	got string,
) FieldError {
	return FieldError{
		Key:     key,
		Message: fmt.Sprintf("must be a positive duration such as 30s or 5m, got %q", got),
	}
}

func atLeastOneError(
	key string,
	got int,
) FieldError {
	return FieldError{
		Key:     key,
		Message: fmt.Sprintf("must be at least 1, got %d", got),
	}
}

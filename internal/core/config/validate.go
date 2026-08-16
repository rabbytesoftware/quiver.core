package config

import "fmt"

const (
	keyPortStart = "netbridge.ephemeral_port_start"
	keyPortEnd   = "netbridge.ephemeral_port_end"
)

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

	for _, f := range Fields() {
		if fe := f.Check(data); fe != nil {
			errs = append(errs, *fe)
		}
	}

	return append(errs, portRangeErrors(data)...)
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
		RestoreField(data, def, fe.Key)
	}

	return corrected
}

// RestoreField copies a single field, addressed by its dotted key, from src
// into data. An unrecognised key is ignored. It is how the load-time sanitize
// pass undoes one field without disturbing its siblings.
func RestoreField(
	data *ConfigData,
	src ConfigData,
	key string,
) {
	for _, f := range Fields() {
		if f.Key() == key {
			f.Restore(data, src)
			return
		}
	}
}

// portRangeErrors reports the one rule spanning two fields. It stays out of
// the field table because no single field owns it, and it is reported against
// the end bound so that restoring one default resolves it.
func portRangeErrors(
	data ConfigData,
) []FieldError {
	start := data.Netbridge.EphemeralPortStart
	end := data.Netbridge.EphemeralPortEnd

	if !validPort(start) || !validPort(end) || start <= end {
		return nil
	}

	return []FieldError{{
		Key: keyPortEnd,
		Message: fmt.Sprintf(
			"must be greater than or equal to netbridge.ephemeral_port_start (%d), got %d",
			start, end,
		),
	}}
}

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

// portRangeErrors reports the one rule spanning two fields. It stays out of
// the validate tags because expressing it there would put a Go field name
// into the API's error text, and it is reported against the end bound so that
// restoring a single default resolves it.
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
			"must be greater than or equal to %s (%d), got %d",
			keyPortStart, start, end,
		),
	}}
}

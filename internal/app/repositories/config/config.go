// Package config reads and writes the daemon configuration on behalf of the
// app layer. It owns no aggregate and emits no events: the configuration is a
// file, and every change to it takes effect on the next daemon start.
package config

import coreconfig "github.com/rabbytesoftware/quiver.core/internal/core/config"

type (
	// Data is a complete configuration document.
	Data = coreconfig.ConfigData

	// FieldError names a configuration field that failed validation.
	FieldError = coreconfig.FieldError
)

// Config is the daemon configuration as the app layer sees it.
type Config interface {
	// Running returns the configuration this process booted with.
	Running() Data

	// Configured returns what the next start will use, read fresh from disk.
	// It differs from Running whenever a change is waiting on a restart.
	Configured() (Data, error)

	// Defaults returns the configuration compiled into the binary.
	Defaults() Data

	// Validate reports every field the daemon cannot use.
	Validate(data Data) []FieldError

	// Save persists the configuration for the next start.
	Save(data Data) error
}

type repository struct{}

// New returns the Config repository backed by the real configuration file and
// the running process singleton.
func New() Config {
	return repository{}
}

func (repository) Running() Data {
	return coreconfig.Get().Config
}

func (repository) Configured() (Data, error) {
	data, _, err := coreconfig.Configured()
	return data, err
}

func (repository) Defaults() Data {
	return coreconfig.Defaults()
}

func (repository) Validate(
	data Data,
) []FieldError {
	return coreconfig.Validate(data)
}

func (repository) Save(
	data Data,
) error {
	return coreconfig.Save(data)
}

// RestoreField copies a single field, addressed by its dotted key, from src
// into data. An unrecognised key is ignored. It is how a caller undoes one
// field without disturbing its siblings.
func RestoreField(
	data *Data,
	src Data,
	key string,
) {
	coreconfig.RestoreField(data, src, key)
}

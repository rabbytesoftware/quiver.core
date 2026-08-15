package usecases

import "github.com/rabbytesoftware/quiver.core/internal/core/config"

// ConfigStore is the configuration state the usecase reads and writes. It
// exists so the usecase can be exercised without a real home directory.
type ConfigStore interface {
	// Running returns the configuration this process booted with.
	Running() config.ConfigData

	// Configured returns what the next start will use, read fresh from disk.
	Configured() (config.ConfigData, error)

	// Defaults returns the configuration compiled into the binary.
	Defaults() config.ConfigData

	// Validate reports every field the daemon cannot use.
	Validate(data config.ConfigData) []config.FieldError

	// Save persists the configuration for the next start.
	Save(data config.ConfigData) error
}

type coreConfigStore struct{}

// NewCoreConfigStore returns the ConfigStore backed by the real configuration
// file and the running process singleton.
func NewCoreConfigStore() ConfigStore {
	return coreConfigStore{}
}

func (coreConfigStore) Running() config.ConfigData {
	return config.Get().Config
}

func (coreConfigStore) Configured() (config.ConfigData, error) {
	data, _, err := config.Configured()
	return data, err
}

func (coreConfigStore) Defaults() config.ConfigData {
	return config.Defaults()
}

func (coreConfigStore) Validate(
	data config.ConfigData,
) []config.FieldError {
	return config.Validate(data)
}

func (coreConfigStore) Save(
	data config.ConfigData,
) error {
	return config.Save(data)
}

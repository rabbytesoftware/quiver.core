// Package paths resolves named Quiver directories from metadata, creates them
// on demand with a per-path mutex, and returns their absolute paths.
//
// Config (config.yaml) is intentionally absent — it is a file, not a directory,
// and is owned by the config package via metadata.GetConfigPath().
package paths

import (
	"fmt"
	"os"
	"sync"

	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
)

// mu stores one *sync.Mutex per absolute path, created on first access.
// Serializes concurrent directory creation for the same path.
var mu sync.Map

// ensure creates dir at path if it does not exist and returns path.
// Concurrent calls for the same path are serialized by a per-path mutex.
func ensure(
	path string,
) (string, error) {
	v, _ := mu.LoadOrStore(path, &sync.Mutex{})
	m := v.(*sync.Mutex)
	m.Lock()
	defer m.Unlock()
	if err := os.MkdirAll(path, 0o750); err != nil {
		return "", fmt.Errorf("paths: create %q: %w", path, err)
	}
	return path, nil
}

// Events returns the absolute path to the event-store directory,
// creating it if it does not exist.
func Events() (string, error) {
	return ensure(
		metadata.GetEventsPath(),
	)
}

// EventsAt returns the absolute path to the event-store directory rooted at
// homeDir instead of the process-level HOME, creating it if it does not exist.
func EventsAt(homeDir string) (string, error) {
	return ensure(
		metadata.GetEventsPathAt(homeDir),
	)
}

// Store returns the absolute path to the catalog read-model directory,
// creating it if it does not exist.
func Store() (string, error) {
	return ensure(
		metadata.GetStorePath(),
	)
}

// StoreAt returns the absolute path to the catalog read-model directory rooted at
// homeDir instead of the process-level HOME, creating it if it does not exist.
func StoreAt(homeDir string) (string, error) {
	return ensure(
		metadata.GetStorePathAt(homeDir),
	)
}

// Namespaces returns the absolute path to the namespaces directory,
// creating it if it does not exist.
func Namespaces() (string, error) {
	return ensure(
		metadata.GetNamespacesPath(),
	)
}

func NamespacesAt(homeDir string) (string, error) {
	return ensure(metadata.GetNamespacesPathAt(homeDir))
}

// Logs returns the absolute path to the logs directory,
// creating it if it does not exist.
func Logs() (string, error) {
	return ensure(
		metadata.GetLogsPath(),
	)
}

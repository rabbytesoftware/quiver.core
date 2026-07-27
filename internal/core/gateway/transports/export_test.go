package transports

import "os"

// SetChmod replaces the chmod function used by socketTransport for testing.
// Returns a restore function the caller must defer.
func SetChmod(fn func(string, os.FileMode) error) func() {
	prev := chmod
	chmod = fn
	return func() { chmod = prev }
}

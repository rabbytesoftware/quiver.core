package step

// ProcessTracker is the narrow interface step handlers use to record the
// key of the OS process they start and to look up that key for signalling.
// The wizard owns the concrete implementation and passes it on each Request.
type ProcessTracker interface {
	// SetKey records the process key after the process has been started.
	SetKey(key string)

	// GetKey returns the process key and true if one has been set,
	// or "", false if no process has been started yet.
	GetKey() (string, bool)
}

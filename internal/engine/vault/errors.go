package vault

import "errors"

var (
	ErrNotCached        = errors.New("vault: manifest not cached")
	ErrStale            = errors.New("vault: manifest cache is stale")
	ErrInvalidNamespace = errors.New("vault: invalid namespace")
	ErrClosed           = errors.New("vault: index is closed")
	// ErrConfirmedAbsent reports that a previous resolution attempt for this
	// exact namespace definitively found no manifest (a real ErrNotFound,
	// not a transient failure), and that finding is still within its TTL.
	// The caller should treat this the same as a live not-found rather than
	// attempting another fetch.
	ErrConfirmedAbsent = errors.New("vault: manifest confirmed absent")
)

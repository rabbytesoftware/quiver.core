package vault

import "errors"

var (
	ErrNotCached        = errors.New("vault: manifest not cached")
	ErrStale            = errors.New("vault: manifest cache is stale")
	ErrInvalidNamespace = errors.New("vault: invalid namespace")
	ErrClosed           = errors.New("vault: index is closed")
	// ErrWorkDirCollision reports an identity whose workdir path is, on a
	// case-insensitive filesystem, another identity's directory from an
	// earlier layout. Handing it out would let one uninstall delete the
	// other's files.
	ErrWorkDirCollision = errors.New("vault: workdir collides with another identity")
	// ErrConfirmedAbsent reports that a previous resolution attempt for this
	// exact namespace definitively found no manifest (a real ErrNotFound,
	// not a transient failure), and that finding is still within its TTL.
	// The caller should treat this the same as a live not-found rather than
	// attempting another fetch.
	ErrConfirmedAbsent = errors.New("vault: manifest confirmed absent")
)

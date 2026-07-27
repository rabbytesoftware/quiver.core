package vault

import "errors"

var (
	ErrNotCached        = errors.New("vault: manifest not cached")
	ErrStale            = errors.New("vault: manifest cache is stale")
	ErrInvalidNamespace = errors.New("vault: invalid namespace")
	ErrClosed           = errors.New("vault: index is closed")
)

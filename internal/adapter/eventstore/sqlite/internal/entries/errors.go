package entries

import "errors"

// ErrVersionConflict marks an Append that lost an optimistic-concurrency race:
// another writer already appended this (aggregate, version), so nothing was
// written and the command can be sent again from scratch.
var ErrVersionConflict = errors.New("version conflict")

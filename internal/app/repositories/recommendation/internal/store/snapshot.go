package store

import "time"

// Snapshot is one shelf as last refreshed. RefreshedAt is the zero time for a
// shelf that has never been filled.
type Snapshot struct {
	RefreshedAt time.Time
	Entries     []Entry
}

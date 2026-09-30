package recommendationinternal

import "time"

// Source is one query-less search a shelf is built from.
type Source struct {
	Host         string
	Sort         string
	MinStars     int
	MaxStars     int
	PushedWithin time.Duration
}

// Shelf is a named group of sources, already validated.
type Shelf struct {
	ID      string
	Title   string
	Limit   int
	Sources []Source
}

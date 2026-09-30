package models

import "time"

// HomeShelf is one named list of arrows on the home screen. RefreshedAt is the
// zero time for a shelf that has never been filled.
type HomeShelf struct {
	ID          string
	Title       string
	RefreshedAt time.Time
	Arrows      []SearchResult
}

// Home is what the home screen shows: its shelves, and whether a refresh is
// running that may still change them.
type Home struct {
	Shelves    []HomeShelf
	Refreshing bool
}

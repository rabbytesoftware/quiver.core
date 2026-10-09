package discovery

import "time"

// BrowseSource is one query-less search against the hosts that answer to Host.
// Host names a git host either by its full name or by its first label, so
// "github" reaches "github.com".
//
// MaxStars is an upper star bound where zero means unbounded; hosts whose
// search has no such filter ignore it.
type BrowseSource struct {
	Host         string
	Sort         string
	MinStars     int
	MaxStars     int
	PushedWithin time.Duration
	Limit        int
}

// BrowseRequest asks for the best repositories of one or more sources, merged
// into a single list. Budget caps how many candidates the pass may resolve that
// the vault does not already hold: those are the only ones that cost a metered
// host request, and is a hard cap. Zero resolves none. Want is the number of
// results the caller is after: once the vault's hits plus the newly verified
// ones reach it no further candidate is dispatched, though resolves already in
// flight finish. Zero resolves up to Budget regardless.
type BrowseRequest struct {
	Sources []BrowseSource
	Budget  int
	Want    int
	// Concurrency caps how many candidates are resolved at once; zero uses the
	// pipeline's own. A pass nobody is waiting on sets it low, so it cannot
	// starve the requests someone is.
	Concurrency int
}

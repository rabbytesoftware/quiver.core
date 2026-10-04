package logring

import "log/slog"

// Ring is a bounded, concurrency-safe buffer of the daemon's recent log
// records with live fan-out to subscribers.
type Ring interface {
	// Snapshot returns up to limit of the most recent retained records whose
	// sequence number is greater than since and whose level is at least min,
	// oldest first. A limit below one returns nothing.
	Snapshot(
		since uint64,
		min slog.Level,
		limit int,
	) []Record

	// Latest returns the sequence number of the newest record, or zero when
	// nothing has been captured yet.
	Latest() uint64

	// Subscribe registers a live subscriber that receives every record added
	// from now on whose level is at least min. The caller must Close it.
	Subscribe(
		min slog.Level,
	) Subscription

	// Tee wraps next so every record it handles is also captured in the ring.
	// next keeps receiving every record exactly as before.
	Tee(
		next slog.Handler,
	) slog.Handler
}

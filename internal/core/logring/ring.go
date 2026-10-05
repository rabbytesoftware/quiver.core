package logring

import "log/slog"

// Ring is a fixed-size, concurrency-safe store of the daemon's latest log
// records that can be replayed and followed.
type Ring interface {
	// Wrap returns a slog.Handler that stores every record next handles, then
	// passes the record on to next and returns next's error.
	//
	// The ring sees exactly the records next is enabled for, so the log level
	// that applies to the daemon's own output applies to the ring too. The
	// logger calls Wrap itself when it is given logger.WithRing.
	Wrap(next slog.Handler) slog.Handler

	// Stream opens a Stream of the records logged at level or above.
	//
	// since is the Seq of the last record the caller already has. With since
	// zero the replay is the last 500 records. With since set it is every
	// retained record after it, and since newer than the newest Seq (the
	// daemon restarted) is treated as zero and flagged by Stream.Reset. The
	// replay and the live channel are cut at the same instant, so no record is
	// missed or repeated between them. Close the stream when done.
	Stream(since uint64, level slog.Level) Stream
}

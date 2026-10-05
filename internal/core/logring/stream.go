package logring

// Stream is a gap-free view of a Ring: the records to replay, then the live
// ones that follow.
type Stream interface {
	// Replay returns the retained records to send first, oldest first.
	Replay() []Record

	// Seq returns the newest sequence number the ring held when the stream
	// opened, which is where a client resumes from.
	Seq() uint64

	// Reset reports that the since the caller passed was newer than Seq,
	// so the daemon restarted and the replay holds the last 500 records.
	Reset() bool

	// Live delivers each record logged after the stream opened. The channel is
	// closed by Close, or by the ring when the reader falls 256 records behind,
	// in which case the reader should reconnect with since set to the last Seq
	// it received.
	Live() <-chan Record

	// Close unsubscribes the stream and closes Live. It is safe to call more
	// than once.
	Close()
}

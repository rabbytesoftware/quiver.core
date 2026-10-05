package logring

type openStream struct {
	ring       *ringBuffer
	subscriber *subscriber
	replay     []Record
	seq        uint64
	reset      bool
}

// Replay returns the retained records to send first, oldest first.
func (s *openStream) Replay() []Record {
	return s.replay
}

// Seq returns the newest sequence number the ring held when the stream opened.
func (s *openStream) Seq() uint64 {
	return s.seq
}

// Reset reports that the requested since was newer than the ring's newest Seq.
func (s *openStream) Reset() bool {
	return s.reset
}

// Live delivers each record logged after the stream opened.
func (s *openStream) Live() <-chan Record {
	return s.subscriber.records
}

// Close unsubscribes the stream and closes the live channel; calling it again
// does nothing.
func (s *openStream) Close() {
	s.ring.unsubscribe(s.subscriber)
}

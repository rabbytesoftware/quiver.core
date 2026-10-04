package logring

import "time"

// Subscription is one live consumer of a Ring. The ring never blocks on a
// subscriber: when its queue is full, new records are dropped and counted.
type Subscription interface {
	// C delivers records in sequence order. It is closed by Close.
	C() <-chan Record

	// TakeDropped returns the number of records dropped since the last call
	// and resets the count.
	TakeDropped() uint64

	// StuckFor reports how long the queue has been continuously full, or zero
	// when it is accepting records.
	StuckFor() time.Duration

	// Close unregisters the subscription and closes C. It is safe to call
	// more than once.
	Close()
}

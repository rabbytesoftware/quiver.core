package discovery

import "sync"

// stream serialises the caller's callback, so an emit written for one goroutine
// stays correct while the fetch pool runs many.
type stream struct {
	mu   sync.Mutex
	emit func(Result)
}

func newStream(
	emit func(Result),
) *stream {
	return &stream{emit: emit}
}

func (s *stream) send(
	result Result,
) {
	if s.emit == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.emit(result)
}

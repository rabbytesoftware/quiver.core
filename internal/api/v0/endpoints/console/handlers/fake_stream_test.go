package console

import (
	"sync"

	"github.com/rabbytesoftware/quiver.core/internal/core/logring"
)

type fakeStream struct {
	live   chan logring.Record
	closed chan struct{}
	once   sync.Once
}

func newFakeStream() *fakeStream {
	return &fakeStream{live: make(chan logring.Record), closed: make(chan struct{})}
}

func (s *fakeStream) Replay() []logring.Record {
	return nil
}

func (s *fakeStream) Seq() uint64 {
	return 0
}

func (s *fakeStream) Reset() bool {
	return false
}

func (s *fakeStream) Live() <-chan logring.Record {
	return s.live
}

func (s *fakeStream) Close() {
	s.once.Do(func() {
		close(s.closed)
	})
}

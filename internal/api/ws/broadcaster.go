package ws

import (
	"log/slog"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
)

type frame struct {
	seq  uint64
	data []byte
}

type filteredClient[T any] struct {
	*client
	predicate func(T) bool

	mu      sync.Mutex
	holding bool
	floor   uint64
	pending []frame
}

type Broadcaster[T any] struct {
	def        StreamDef[T]
	mu         sync.RWMutex
	clients    map[*filteredClient[T]]struct{}
	registered chan struct{}
	once       sync.Once
}

func NewBroadcaster[T any](def StreamDef[T]) *Broadcaster[T] {
	return &Broadcaster[T]{
		def:        def,
		clients:    make(map[*filteredClient[T]]struct{}),
		registered: make(chan struct{}),
	}
}

func (b *Broadcaster[T]) WaitRegistered() { <-b.registered }

func (b *Broadcaster[T]) Handle(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	cl := &filteredClient[T]{
		client:    newClient(),
		predicate: BuildPredicate(c, b.def),
		holding:   b.def.Replay != nil,
	}

	b.mu.Lock()
	b.clients[cl] = struct{}{}
	b.once.Do(func() { close(b.registered) })
	b.mu.Unlock()

	go writePump(conn, cl.client)

	key := b.key(c)
	b.replay(cl, key)
	go b.finishWhenDone(cl, key)
	readPump(conn)

	b.mu.Lock()
	delete(b.clients, cl)
	close(cl.done)
	b.mu.Unlock()
}

func (b *Broadcaster[T]) key(c *gin.Context) string {
	param := b.def.KeyParam
	if param == "" {
		param = defaultKeyParam
	}
	return c.Param(param)
}

// replay sends a new subscriber everything its key emitted before it
// registered, then releases the live frames held back meanwhile. Replay frames
// wait for buffer space instead of overflowing it: a backlog longer than the
// buffer is the normal case for a late subscriber, not a slow one.
func (b *Broadcaster[T]) replay(
	cl *filteredClient[T],
	key string,
) {
	if b.def.Replay == nil {
		return
	}

	var floor uint64
	for _, event := range b.def.Replay(key) {
		floor = max(floor, b.seq(event))
		if !cl.predicate(event) {
			continue
		}
		data, err := b.def.Serialize(event)
		if err != nil {
			continue
		}
		select {
		case cl.send <- data:
		case <-cl.dead:
			return
		}
	}

	cl.mu.Lock()
	defer cl.mu.Unlock()
	cl.floor = floor
	cl.holding = false
	for _, f := range cl.pending {
		if f.seq == 0 || f.seq > floor {
			b.enqueue(cl, f.data)
		}
	}
	cl.pending = nil
}

func (b *Broadcaster[T]) finishWhenDone(
	cl *filteredClient[T],
	key string,
) {
	if b.def.Done == nil {
		return
	}
	select {
	case <-b.def.Done(key):
		cl.requestFinish()
	case <-cl.dead:
	}
}

func (b *Broadcaster[T]) seq(event T) uint64 {
	if b.def.Seq == nil {
		return 0
	}
	return b.def.Seq(event)
}

func (b *Broadcaster[T]) Push(event T) {
	data, err := b.def.Serialize(event)
	if err != nil {
		return
	}
	seq := b.seq(event)

	b.mu.RLock()
	defer b.mu.RUnlock()
	for cl := range b.clients {
		if cl.predicate(event) {
			b.deliver(cl, frame{seq: seq, data: data})
		}
	}
}

func (b *Broadcaster[T]) deliver(
	cl *filteredClient[T],
	f frame,
) {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	if f.seq != 0 && f.seq <= cl.floor {
		return
	}
	if cl.holding {
		cl.pending = append(cl.pending, f)
		return
	}
	b.enqueue(cl, f.data)
}

func (b *Broadcaster[T]) enqueue(
	cl *filteredClient[T],
	data []byte,
) {
	select {
	case cl.send <- data:
		return
	default:
	}

	if b.def.Overflow == OverflowDisconnect {
		slog.Warn("ws: closing slow consumer; it can reconnect and be replayed")
		cl.requestKick()
		return
	}
	slog.Warn("ws: dropped frame for slow consumer")
}

package ws

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
)

type filteredClient[T any] struct {
	*client
	predicate func(T) bool
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

	predicate := BuildPredicate(c, b.def)
	cl := &filteredClient[T]{client: newClient(), predicate: predicate}

	b.mu.Lock()
	b.clients[cl] = struct{}{}
	b.once.Do(func() { close(b.registered) })
	b.mu.Unlock()

	go writePump(conn, cl.client)
	readPump(conn)

	b.mu.Lock()
	delete(b.clients, cl)
	close(cl.done)
	b.mu.Unlock()
}

func (b *Broadcaster[T]) Push(event T) {
	data, err := b.def.Serialize(event)
	if err != nil {
		return
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for cl := range b.clients {
		if cl.predicate(event) {
			select {
			case cl.send <- data:
			default:
			}
		}
	}
}

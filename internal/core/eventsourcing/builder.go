package eventsourcing

type EventTypeRegistration[T Event] struct {
	EventType string
	Factory   EventFactory[T]
}

type Builder[T Event] struct {
	store         EventStore[T]
	bus           EventBus
	retentionDays int
	autoReplay    bool
	eventTypes    []EventTypeRegistration[T]
}

func NewBuilder[T Event]() *Builder[T] {
	return &Builder[T]{
		retentionDays: 30,
		autoReplay:    false,
	}
}

func (b *Builder[T]) WithStore(store EventStore[T]) *Builder[T] {
	b.store = store
	return b
}

func (b *Builder[T]) WithBus(bus EventBus) *Builder[T] {
	b.bus = bus
	return b
}

func (b *Builder[T]) WithRetentionDays(days int) *Builder[T] {
	b.retentionDays = days
	return b
}

func (b *Builder[T]) WithAutoReplay(enabled bool) *Builder[T] {
	b.autoReplay = enabled
	return b
}

func (b *Builder[T]) WithEventTypes(types []EventTypeRegistration[T]) *Builder[T] {
	b.eventTypes = types
	return b
}

func (b *Builder[T]) Build() *EventSourcing[T] {
	registry := NewRegistry[T]()
	for _, reg := range b.eventTypes {
		registry.Register(reg.EventType, reg.Factory)
	}

	return NewEventSourcing(b.store, b.bus, registry)
}

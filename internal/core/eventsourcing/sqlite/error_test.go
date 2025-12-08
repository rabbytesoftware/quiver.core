package sqlite_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing"
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/sqlite"
	"github.com/stretchr/testify/assert"
)

type UnmarshallableEvent struct {
	eventsourcing.BaseEvent
	BadField chan int
}

func TestSQLiteEventStore_AppendWithMarshalError(t *testing.T) {
	ctx := context.Background()

	dbName := "test_marshal_error"
	dbPath := filepath.Join("./db", dbName+".db")
	os.Remove(dbPath)
	t.Cleanup(func() {
		os.Remove(dbPath)
	})

	registry := eventsourcing.NewRegistry[eventsourcing.Event]()
	store := sqlite.NewEventStore[eventsourcing.Event](ctx, dbName, registry)

	event := &UnmarshallableEvent{
		BaseEvent: eventsourcing.NewBaseEvent("agg_999", "test", "test.unmarshal"),
		BadField:  make(chan int),
	}

	err := store.Append(ctx, event)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to marshal event")
}

type BadJSONEvent struct {
	eventsourcing.BaseEvent
}

func (b *BadJSONEvent) MarshalJSON() ([]byte, error) {
	_, err := json.Marshal(make(chan int))
	return nil, err
}

func TestSQLiteEventStore_AppendWithCustomMarshalError(t *testing.T) {
	ctx := context.Background()

	dbName := "test_custom_marshal_error"
	dbPath := filepath.Join("./db", dbName+".db")
	os.Remove(dbPath)
	t.Cleanup(func() {
		os.Remove(dbPath)
	})

	registry := eventsourcing.NewRegistry[eventsourcing.Event]()
	store := sqlite.NewEventStore[eventsourcing.Event](ctx, dbName, registry)

	event := &BadJSONEvent{
		BaseEvent: eventsourcing.NewBaseEvent("agg_888", "test", "test.badjson"),
	}

	err := store.Append(ctx, event)
	assert.Error(t, err)
}


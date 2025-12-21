package idempotency

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Record struct {
	ID            uuid.UUID
	EventType     string
	EventPayload  string
	CorrelationID uuid.UUID
	Response      string
	CreatedAt     time.Time
	ExpiresAt     time.Time
}

type Store struct {
	mu      sync.RWMutex
	records map[uuid.UUID]*Record
}

func NewStore() *Store {
	return &Store{
		records: make(map[uuid.UUID]*Record),
	}
}

func (s *Store) Exists(
	ctx context.Context, 
	key uuid.UUID,
) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	record, exists := s.records[key]
	if !exists {
		return false, nil
	}

	if time.Now().After(record.ExpiresAt) {
		return false, nil
	}

	return true, nil
}

func (s *Store) Get(
	ctx context.Context, 
	key uuid.UUID,
) (*Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	record, exists := s.records[key]
	if !exists {
		return nil, fmt.Errorf("idempotency record not found")
	}

	if time.Now().After(record.ExpiresAt) {
		return nil, fmt.Errorf("idempotency record expired")
	}

	return record, nil
}

func (s *Store) Set(ctx context.Context, record *Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.records[record.ID] = record
	return nil
}

func (s *Store) CheckAndSkipIfExists(
	ctx context.Context, 
	namespace, aggregateID, aggregateType, eventType string, 
	metadata map[string]interface{},
) (bool, error) {
	key := generateKey(namespace, aggregateID, aggregateType, eventType, metadata)
	return s.Exists(ctx, key)
}

func (s *Store) RecordEvent(
	ctx context.Context, 
	namespace, aggregateID, aggregateType, eventType, correlationID string, 
	metadata map[string]interface{}, 
	ttl time.Duration,
) error {
	key := generateKey(
		namespace, 
		aggregateID, 
		aggregateType, 
		eventType, 
		metadata,
	)
	
	record := &Record{
		ID:            key,
		EventType:     eventType,
		EventPayload:  "",
		CorrelationID: uuid.MustParse(correlationID),
		Response:      "",
		CreatedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(ttl),
	}
	
	return s.Set(ctx, record)
}

func generateKey(
	namespace string, 
	aggregateID, aggregateType, eventType string, 
	metadata map[string]interface{},
) uuid.UUID {
	parts := []string{
		aggregateID,
		aggregateType,
		eventType,
	}

	if len(metadata) > 0 {
		keys := make([]string, 0, len(metadata))
		for k := range metadata {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s=%v", k, metadata[k]))
		}
	}

	idempotencyString := strings.Join(parts, "|")
	namespaceUUID := uuid.MustParse(namespace)
	return uuid.NewSHA1(namespaceUUID, []byte(idempotencyString))
}

package idempotency

import (
	"context"
	"fmt"
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

func (s *Store) Exists(ctx context.Context, key uuid.UUID) (bool, error) {
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

func (s *Store) Get(ctx context.Context, key uuid.UUID) (*Record, error) {
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


package mocks

import (
	"context"
	"fmt"
	"sync"

	"github.com/char2cs/asynx/models"
)

type memoryEntry struct {
	aggregateID string
	version     int64
	data        []byte
}

// MemoryEventStore is a test-only in-memory implementation of models.Store.
type MemoryEventStore struct {
	mu      sync.Mutex
	entries []memoryEntry
}

func NewMemoryEventStore() models.Store {
	return &MemoryEventStore{}
}

func (s *MemoryEventStore) Append(
	ctx context.Context,
	aggregateID string,
	version int64,
	data []byte,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, e := range s.entries {
		if e.aggregateID == aggregateID && e.version == version {
			return fmt.Errorf("%w: version conflict (%s, v%d)", models.ErrPipelineFailed, aggregateID, version)
		}
	}

	s.entries = append(s.entries, memoryEntry{aggregateID: aggregateID, version: version, data: data})
	return nil
}

func (s *MemoryEventStore) ReadFrom(
	ctx context.Context,
	aggregateID string,
	fromVersion int64,
) ([][]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var result [][]byte
	for _, e := range s.entries {
		if e.aggregateID == aggregateID && e.version >= fromVersion {
			result = append(result, e.data)
		}
	}
	return result, nil
}

func (s *MemoryEventStore) ReadRange(
	ctx context.Context,
	aggregateID string,
	fromVersion int64,
	count int64,
) ([][]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var result [][]byte
	for _, e := range s.entries {
		if e.aggregateID == aggregateID && e.version >= fromVersion {
			result = append(result, e.data)
			if int64(len(result)) >= count {
				break
			}
		}
	}
	return result, nil
}

func (s *MemoryEventStore) Count(
	ctx context.Context,
	aggregateID string,
	fromVersion int64,
) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var count int64
	for _, e := range s.entries {
		if e.aggregateID == aggregateID && e.version >= fromVersion {
			count++
		}
	}
	return count, nil
}

func (s *MemoryEventStore) Delete(
	ctx context.Context,
	aggregateID string,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	filtered := s.entries[:0]
	for _, e := range s.entries {
		if e.aggregateID != aggregateID {
			filtered = append(filtered, e)
		}
	}
	s.entries = filtered
	return nil
}

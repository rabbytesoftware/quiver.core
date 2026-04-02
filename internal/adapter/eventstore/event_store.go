package store

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"sync"

	asynxModels "github.com/char2cs/asynx/models"
)

type storeEntry struct {
	version int64
	data    []byte
}

type eventStore struct {
	mu      sync.RWMutex
	streams map[string][]storeEntry
}

// NewEventStore returns an in-memory asynx event store.
func NewEventStore() *eventStore {
	return &eventStore{
		streams: make(map[string][]storeEntry),
	}
}

func (s *eventStore) Append(
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

	entries := s.streams[aggregateID]
	idx, found := slices.BinarySearchFunc(
		entries,
		version,
		func(e storeEntry, v int64) int {
			return cmp.Compare(e.version, v)
		},
	)
	if found {
		return fmt.Errorf("%w: version conflict (%s, v%d)", asynxModels.ErrPipelineFailed, aggregateID, version)
	}

	s.streams[aggregateID] = slices.Insert(entries, idx, storeEntry{version: version, data: data})
	return nil
}

func (s *eventStore) ReadFrom(
	ctx context.Context,
	aggregateID string,
	fromVersion int64,
) ([][]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	return storeEntriesToBlobs(s.entriesFrom(aggregateID, fromVersion)), nil
}

func (s *eventStore) ReadRange(
	ctx context.Context,
	aggregateID string,
	fromVersion int64,
	count int64,
) ([][]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	entries := s.entriesFrom(aggregateID, fromVersion)
	if int64(len(entries)) > count {
		entries = entries[:count]
	}

	return storeEntriesToBlobs(entries), nil
}

func (s *eventStore) Count(
	ctx context.Context,
	aggregateID string,
	fromVersion int64,
) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	return int64(len(s.entriesFrom(aggregateID, fromVersion))), nil
}

func (s *eventStore) entriesFrom(
	aggregateID string,
	fromVersion int64,
) []storeEntry {
	entries := s.streams[aggregateID]
	if len(entries) == 0 {
		return nil
	}

	startIdx, _ := slices.BinarySearchFunc(
		entries,
		fromVersion,
		func(e storeEntry, v int64) int {
			return cmp.Compare(e.version, v)
		},
	)
	return entries[startIdx:]
}

func storeEntriesToBlobs(
	entries []storeEntry,
) [][]byte {
	result := make([][]byte, len(entries))
	for i, e := range entries {
		result[i] = e.data
	}
	return result
}

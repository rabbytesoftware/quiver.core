package projections

import (
	"context"
	"errors"
	"testing"

	"github.com/rabbytesoftware/quiver.core/internal/domain/netbridge"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge/internal/ports"
	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge/internal/store"
)

func TestHandlePortEvent_Allocated(t *testing.T) {
	rm := mocks.NewStubReadModel()
	handler := HandlePortEvent(rm)

	alloc := ports.PortAllocation{
		Port:      8080,
		Protocol:  netbridge.ProtocolTCP,
		OwnerKey:  "owner-1",
		Forwarded: true,
	}

	evt := asynxModels.Event[ports.PortAllocation]{
		EventName: "port.Allocated",
		Aggregate: alloc,
	}

	handler(context.Background(), evt)

	saved, err := rm.FindByPort(context.Background(), 8080)
	assert.NoError(t, err)
	assert.NotNil(t, saved)
	assert.Equal(t, "owner-1", saved.OwnerKey)
	assert.True(t, saved.Forwarded)
}

func TestHandlePortEvent_Deallocated(t *testing.T) {
	rm := mocks.NewStubReadModel()
	handler := HandlePortEvent(rm)

	// Pre-populate with an allocation
	alloc := ports.PortAllocation{
		Port:      9090,
		Protocol:  netbridge.ProtocolUDP,
		OwnerKey:  "owner-2",
		Forwarded: false,
	}
	rm.Save(context.Background(), alloc)

	// Verify it's there
	saved, err := rm.FindByPort(context.Background(), 9090)
	assert.NoError(t, err)
	assert.NotNil(t, saved)

	// Fire deallocate event
	evt := asynxModels.Event[ports.PortAllocation]{
		EventName:         "port.Deallocated",
		PreviousAggregate: alloc,
	}

	handler(context.Background(), evt)

	// Verify it's been deleted
	deleted, err := rm.FindByPort(context.Background(), 9090)
	assert.NoError(t, err)
	assert.Nil(t, deleted)
}

func TestHandlePortEvent_UnknownEvent(t *testing.T) {
	rm := mocks.NewStubReadModel()
	handler := HandlePortEvent(rm)

	alloc := ports.PortAllocation{
		Port:      7070,
		Protocol:  netbridge.ProtocolTCPUDP,
		OwnerKey:  "owner-3",
		Forwarded: true,
	}

	evt := asynxModels.Event[ports.PortAllocation]{
		EventName: "port.UnknownEvent",
		Aggregate: alloc,
	}

	handler(context.Background(), evt)

	// Should not have been saved (unknown event name)
	saved, err := rm.FindByPort(context.Background(), 7070)
	assert.NoError(t, err)
	assert.Nil(t, saved)
}

type stubHostedStore struct {
	store.HostedPortStore
	saved   []ports.PortAllocation
	deleted []int
	err     error
}

func (s *stubHostedStore) SaveTx(_ context.Context, _ asynxModels.Tx, a ports.PortAllocation) error {
	s.saved = append(s.saved, a)
	return s.err
}

func (s *stubHostedStore) DeleteTx(_ context.Context, _ asynxModels.Tx, port int) error {
	s.deleted = append(s.deleted, port)
	return s.err
}

func TestSyncPortEvent(t *testing.T) {
	boom := errors.New("boom")
	testCases := []struct {
		name        string
		event       string
		storeErr    error
		wantSaved   int
		wantDeleted int
		wantErr     error
	}{
		{name: "allocated saves", event: "port.Allocated", wantSaved: 1},
		{name: "deallocated deletes the previous port", event: "port.Deallocated", wantDeleted: 1},
		{name: "other event ignored", event: "port.Other"},
		{name: "store error aborts", event: "port.Allocated", storeErr: boom, wantSaved: 1, wantErr: boom},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			st := &stubHostedStore{err: tc.storeErr}
			err := SyncPortEvent(st)(context.Background(), nil, asynxModels.Event[ports.PortAllocation]{
				EventName:         tc.event,
				Aggregate:         ports.PortAllocation{Port: 7},
				PreviousAggregate: ports.PortAllocation{Port: 7},
			})
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
			} else {
				assert.NoError(t, err)
			}
			assert.Len(t, st.saved, tc.wantSaved)
			assert.Len(t, st.deleted, tc.wantDeleted)
		})
	}
}

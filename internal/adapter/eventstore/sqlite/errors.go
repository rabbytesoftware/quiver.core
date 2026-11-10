package sqlite

import (
	"errors"

	"github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite/internal/entries"
)

// ErrVersionConflict marks an Append that lost an optimistic-concurrency race:
// another writer already appended this (aggregate, version), so nothing was
// written and the command can be sent again from scratch.
var ErrVersionConflict = entries.ErrVersionConflict

// ErrNotSQLTx is returned by TxSession for a Tx that is not backed by a
// database/sql transaction.
var ErrNotSQLTx = errors.New("eventstore: tx is not backed by a database/sql transaction")

package sqlite

import (
	"context"
	"errors"

	"github.com/char2cs/asynx/models"
	"gorm.io/gorm"
)

// ErrNotSQLTx is returned by TxSession for a Tx that is not backed by a
// database/sql transaction.
var ErrNotSQLTx = errors.New("eventstore: tx is not backed by a database/sql transaction")

// TxSession returns a handle on db whose statements run inside tx, so a sync
// projection's writes commit or roll back with the event.
func TxSession(
	ctx context.Context,
	db *gorm.DB,
	tx models.Tx,
) (*gorm.DB, error) {
	pool, ok := tx.Native().(gorm.ConnPool)
	if !ok {
		return nil, ErrNotSQLTx
	}
	session := db.Session(&gorm.Session{NewDB: true, Context: ctx})
	session.Statement.ConnPool = pool
	return session, nil
}

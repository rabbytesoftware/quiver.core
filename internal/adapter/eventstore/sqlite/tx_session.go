package sqlite

import (
	"context"

	"github.com/char2cs/asynx/models"
	"gorm.io/gorm"
)

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

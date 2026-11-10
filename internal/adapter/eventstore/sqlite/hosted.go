package sqlite

import "gorm.io/gorm"

// Hosted is implemented by a Store that lets read models live in its
// database, so a sync projection can commit with the event it projects.
type Hosted interface {
	// DB returns the events database handle. It is pinned to one connection,
	// so inside a sync projection a write must go through TxSession, never
	// through DB.
	DB() *gorm.DB
}

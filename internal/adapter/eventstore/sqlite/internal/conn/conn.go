package conn

import (
	"fmt"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open opens the SQLite file at path with the settings every store shares:
// silent logging and driver errors translated to gorm's.
func Open(
	path string,
) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger:         logger.Default.LogMode(logger.Silent),
		TranslateError: true,
	})
	if err != nil {
		return nil, fmt.Errorf("conn: open: %w", err)
	}
	return db, nil
}

// Close releases a handle whose constructor failed after Open succeeded, best
// effort. Without it the pooled connection stays open with nothing left to
// close it, since no store was returned to the caller.
func Close(
	db *gorm.DB,
) {
	sqlDB, err := db.DB()
	if err != nil {
		return
	}
	_ = sqlDB.Close()
}

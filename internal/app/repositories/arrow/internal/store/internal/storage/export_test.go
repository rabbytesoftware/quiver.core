package storage

import gormdb "gorm.io/gorm"

// NewSchema exposes the unexported newSchema function for white-box tests.
func NewSchema(db *gormdb.DB) error {
	return newSchema(db)
}

package storage

import gormdb "gorm.io/gorm"

// Unmarshal exposes the unexported unmarshal function for white-box tests.
func Unmarshal(data []byte) (*ViewModel, error) {
	return unmarshal(data)
}

// NewSchema exposes the unexported newSchema function for white-box tests.
func NewSchema(db *gormdb.DB) error {
	return newSchema(db)
}

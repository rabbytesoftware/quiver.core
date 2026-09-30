package storage

import "time"

// ShelfRow records when a shelf's snapshot was last swapped in. A shelf with no
// row has never been filled.
type ShelfRow struct {
	ShelfID     string    `gorm:"primaryKey;column:shelf_id"`
	RefreshedAt time.Time `gorm:"column:refreshed_at"`
}

func (ShelfRow) TableName() string { return "recommendation_shelves" }

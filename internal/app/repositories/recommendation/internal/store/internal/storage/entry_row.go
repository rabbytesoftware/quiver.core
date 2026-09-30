package storage

// EntryRow is one arrow on a shelf's snapshot. Position is its rank, so the
// shelf reads back in the order it was built.
type EntryRow struct {
	ShelfID   string `gorm:"primaryKey;column:shelf_id"`
	Position  int    `gorm:"primaryKey;column:position"`
	Namespace string `gorm:"column:namespace"`
	Stars     int    `gorm:"column:stars"`
	Source    string `gorm:"column:source"`
}

func (EntryRow) TableName() string { return "recommendation_entries" }

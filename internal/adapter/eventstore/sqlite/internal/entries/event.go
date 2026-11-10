package entries

// Event is the row of the events table: one serialized event of a stream.
type Event struct {
	AggregateID string `gorm:"primaryKey;column:aggregate_id"`
	Version     int64  `gorm:"primaryKey;column:version"`
	Data        []byte `gorm:"not null"`
}

// TableName names the events table.
func (Event) TableName() string {
	return "events"
}

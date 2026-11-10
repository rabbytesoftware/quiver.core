package entries

// Delivery is the row of the deliveries table: an event still owed to the
// durable subscribers.
type Delivery struct {
	Seq         int64  `gorm:"primaryKey;autoIncrement;column:seq"`
	AggregateID string `gorm:"not null;column:aggregate_id;uniqueIndex:ux_deliveries_aggregate_version,priority:1"`
	Version     int64  `gorm:"not null;column:version;uniqueIndex:ux_deliveries_aggregate_version,priority:2"`
}

// TableName names the deliveries table.
func (Delivery) TableName() string {
	return "deliveries"
}

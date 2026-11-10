package sqlite

type deliveryEntry struct {
	Seq         int64  `gorm:"primaryKey;autoIncrement;column:seq"`
	AggregateID string `gorm:"not null;column:aggregate_id;uniqueIndex:ux_deliveries_aggregate_version,priority:1"`
	Version     int64  `gorm:"not null;column:version;uniqueIndex:ux_deliveries_aggregate_version,priority:2"`
}

func (deliveryEntry) TableName() string {
	return "deliveries"
}

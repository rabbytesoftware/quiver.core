package entries

// Snapshot is the row of the snapshots table: the latest snapshot of one
// aggregate.
type Snapshot struct {
	AggregateID string `gorm:"primaryKey;column:aggregate_id"`
	Version     int64  `gorm:"not null;column:version"`
	Data        []byte `gorm:"not null;column:data"`
}

// TableName names the snapshots table.
func (Snapshot) TableName() string {
	return "snapshots"
}

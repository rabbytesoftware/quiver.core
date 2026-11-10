package entries

// Receipt is the row of the receipts table: the outcome of one command.
type Receipt struct {
	CommandID   string `gorm:"primaryKey;column:command_id"`
	Accepted    bool   `gorm:"not null;column:accepted"`
	AggregateID string `gorm:"column:aggregate_id"`
	EventID     string `gorm:"column:event_id"`
	Version     int64  `gorm:"column:version"`
	Rejection   string `gorm:"column:rejection"`
}

// TableName names the receipts table.
func (Receipt) TableName() string {
	return "receipts"
}

package sqlite

type receiptEntry struct {
	CommandID   string `gorm:"primaryKey;column:command_id"`
	Accepted    bool   `gorm:"not null;column:accepted"`
	AggregateID string `gorm:"column:aggregate_id"`
	EventID     string `gorm:"column:event_id"`
	Version     int64  `gorm:"column:version"`
	Rejection   string `gorm:"column:rejection"`
}

func (receiptEntry) TableName() string {
	return "receipts"
}

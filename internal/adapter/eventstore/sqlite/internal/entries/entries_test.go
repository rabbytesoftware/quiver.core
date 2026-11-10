package entries

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTableNames(t *testing.T) {
	assert.Equal(t, "events", Event{}.TableName())
	assert.Equal(t, "snapshots", Snapshot{}.TableName())
	assert.Equal(t, "receipts", Receipt{}.TableName())
	assert.Equal(t, "deliveries", Delivery{}.TableName())
}

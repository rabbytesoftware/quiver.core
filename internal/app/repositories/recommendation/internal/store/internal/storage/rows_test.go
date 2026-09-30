package storage_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/recommendation/internal/store/internal/storage"
)

func TestRows_TableNames(t *testing.T) {
	assert.Equal(t, "recommendation_entries", storage.EntryRow{}.TableName())
	assert.Equal(t, "recommendation_shelves", storage.ShelfRow{}.TableName())
}

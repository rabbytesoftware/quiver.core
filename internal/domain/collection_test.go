package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestCollectionArrowEntry_MutuallyExclusive(t *testing.T) {
	local := domain.CollectionArrowEntry{Path: "servers/cs2"}
	ext := domain.CollectionArrowEntry{Namespace: "github.com/valve/steamcmd"}
	assert.NotEmpty(t, local.Path)
	assert.NotEmpty(t, ext.Namespace)
}

func TestQuiver_HasFollowedAt(t *testing.T) {
	q := domain.Collection{
		Namespace:  domain.Namespace("github.com/char2cs/gaming.quiver"),
		FollowedAt: time.Now(),
	}
	assert.False(t, q.FollowedAt.IsZero())
}

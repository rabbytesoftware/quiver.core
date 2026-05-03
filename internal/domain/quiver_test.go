package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

func TestQuiverArrowEntry_MutuallyExclusive(t *testing.T) {
	local := domain.QuiverArrowEntry{Path: "servers/cs2"}
	ext   := domain.QuiverArrowEntry{Namespace: "github.com/valve/steamcmd"}
	assert.NotEmpty(t, local.Path)
	assert.NotEmpty(t, ext.Namespace)
}

func TestQuiverManifest_HasVersionAndArrows(t *testing.T) {
	m := domain.QuiverManifest{
		Name:    "Gaming Quiver",
		Version: "1.0.0",
		Arrows:  []domain.QuiverArrow{{Namespace: "github.com/valve/steamcmd"}},
	}
	assert.Equal(t, "1.0.0", m.Version)
	assert.Len(t, m.Arrows, 1)
}

func TestQuiver_HasFollowedAt(t *testing.T) {
	q := domain.Quiver{
		Namespace:  domain.Namespace("github.com/char2cs/gaming.quiver"),
		FollowedAt: time.Now(),
	}
	assert.False(t, q.FollowedAt.IsZero())
}

package snapshot

import (
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// entry is one Snapshot result, timestamped so cachedSnapshot can tell a
// still-fresh hit from one due for a live re-check.
type entry struct {
	snap     domain.RefSnapshot
	cachedAt time.Time
}

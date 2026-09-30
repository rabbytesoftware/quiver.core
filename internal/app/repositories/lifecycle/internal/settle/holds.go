package settle

import (
	"sync"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// badgeHolds records the rows whose version check was held while they
// settled, so the settle re-derives their badge before it lets them go.
type badgeHolds struct {
	mu    sync.Mutex
	dirty map[domain.Namespace]bool
}

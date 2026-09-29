package usecases

import (
	"sync"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// updateTargets remembers, per row, the target an update began toward. The
// row's own Available cannot stand in for it: a version check during the
// update may record a newer target the update never ran.
type updateTargets struct {
	mu      sync.Mutex
	targets map[domain.Namespace]domain.Available
}

func newUpdateTargets() *updateTargets {
	return &updateTargets{targets: make(map[domain.Namespace]domain.Available)}
}

func (t *updateTargets) put(
	ns domain.Namespace,
	target domain.Available,
) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.targets[ns] = target
}

func (t *updateTargets) take(
	ns domain.Namespace,
) (domain.Available, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	target, ok := t.targets[ns]
	delete(t.targets, ns)
	return target, ok
}

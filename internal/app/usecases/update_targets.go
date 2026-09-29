package usecases

import (
	"sync"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// updateTargets remembers, per row, the target an update began toward. The
// row's own Available cannot stand in for it: a version check during the
// update may record a newer target the update never ran.
type updateTargets struct {
	mu       sync.Mutex
	next     uint64
	targets  map[domain.Namespace]rememberedTarget
	brackets map[domain.Namespace]*sync.Mutex
}

type rememberedTarget struct {
	target domain.Available
	token  uint64
}

func newUpdateTargets() *updateTargets {
	return &updateTargets{
		targets:  make(map[domain.Namespace]rememberedTarget),
		brackets: make(map[domain.Namespace]*sync.Mutex),
	}
}

// open serializes update brackets of one row and returns the call that
// closes this one.
func (t *updateTargets) open(
	ns domain.Namespace,
) func() {
	t.mu.Lock()
	bracket, ok := t.brackets[ns]
	if !ok {
		bracket = &sync.Mutex{}
		t.brackets[ns] = bracket
	}
	t.mu.Unlock()

	bracket.Lock()
	return bracket.Unlock
}

// put remembers target for ns and returns the undo for a bracket that fails
// to begin: it restores what this put replaced, and only while the entry is
// still this put's own.
func (t *updateTargets) put(
	ns domain.Namespace,
	target domain.Available,
) func() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.next++
	token := t.next
	previous, hadPrevious := t.targets[ns]
	t.targets[ns] = rememberedTarget{target: target, token: token}

	return func() {
		t.mu.Lock()
		defer t.mu.Unlock()
		if current, ok := t.targets[ns]; !ok || current.token != token {
			return
		}
		if hadPrevious {
			t.targets[ns] = previous
			return
		}
		delete(t.targets, ns)
	}
}

func (t *updateTargets) take(
	ns domain.Namespace,
) (domain.Available, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	remembered, ok := t.targets[ns]
	delete(t.targets, ns)
	return remembered.target, ok
}

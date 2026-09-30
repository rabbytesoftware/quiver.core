package usecases

import (
	"context"
	"fmt"
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
	brackets map[domain.Namespace]chan struct{}

	// onWait, when set, is told a bracket is about to wait for another one
	// of the same row; tests use it to interleave brackets deterministically.
	onWait func(ns domain.Namespace)
}

type rememberedTarget struct {
	target domain.Available
	token  uint64
}

func newUpdateTargets() *updateTargets {
	return &updateTargets{
		targets:  make(map[domain.Namespace]rememberedTarget),
		brackets: make(map[domain.Namespace]chan struct{}),
	}
}

// open serializes what moves or begins one row (update brackets, installs,
// catalog advances) and returns the call that closes this one. A caller
// whose ctx ends while it waits gives up.
func (t *updateTargets) open(
	ctx context.Context,
	ns domain.Namespace,
) (func(), error) {
	t.mu.Lock()
	bracket, ok := t.brackets[ns]
	if !ok {
		bracket = make(chan struct{}, 1)
		t.brackets[ns] = bracket
	}
	onWait := t.onWait
	t.mu.Unlock()

	release := func() { <-bracket }
	select {
	case bracket <- struct{}{}:
		return release, nil
	default:
	}

	if onWait != nil {
		onWait(ns)
	}
	select {
	case bracket <- struct{}{}:
		return release, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("wait for update of %s: %w", ns, ctx.Err())
	}
}

// pending reports whether an update of ns began and its end has not been
// handled yet.
func (t *updateTargets) pending(
	ns domain.Namespace,
) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, ok := t.targets[ns]
	return ok
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

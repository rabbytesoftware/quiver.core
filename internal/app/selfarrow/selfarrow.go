package selfarrow

import (
	"context"
	"fmt"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// Namespace is the arrow the running quiver.core binary registers itself
// under. It carries no ref by itself — EnsureRegistered appends the running
// build's own version, since that is the ref this process is already at,
// not one for the manifold to resolve.
const Namespace domain.Namespace = "github.com/rabbytesoftware/quiver.core"

// arrowCatalog is the subset of the arrow catalog EnsureRegistered needs.
// Both the repository-level arrow.Arrow the app container passes and this
// package's tests satisfy it structurally, so EnsureRegistered depends on
// neither directly.
type arrowCatalog interface {
	Exists(
		ctx context.Context,
		ns domain.Namespace,
	) (bool, error)
	Add(
		ctx context.Context,
		ns domain.Namespace,
	) error
}

// EnsureRegistered adds quiver.core to its own arrow catalog on first boot at
// a given version, so its own drift can be checked and updated through the
// exact same path as any other arrow. A no-op once already registered, and a
// no-op entirely for an unstamped dev build (version "dev" is not a
// resolvable ref any drift check could compare against).
func EnsureRegistered(
	ctx context.Context,
	arrows arrowCatalog,
	version string,
) error {
	if version == "dev" {
		return nil
	}

	ns := domain.Namespace(fmt.Sprintf("%s@%s", Namespace, version))

	exists, err := arrows.Exists(ctx, ns)
	if err != nil {
		return fmt.Errorf("selfarrow: ensure registered: %w", err)
	}
	if exists {
		return nil
	}

	if err := arrows.Add(ctx, ns); err != nil {
		return fmt.Errorf("selfarrow: ensure registered: %w", err)
	}
	return nil
}

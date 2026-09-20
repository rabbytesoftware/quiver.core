package runtime

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx"

	runtimeinternal "github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal/assembler"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	wizardPkg "github.com/rabbytesoftware/quiver.core/internal/engine/wizard"
)

// Re-export for tests.
type ResolvedExecution = assembler.ResolvedExecution

// NewTestable creates a Runtime with an injected Assembler, for unit testing.
// The badge reconcile is variadic so the tests that predate it — every test
// that does not care what an execution's end does to the outdated badge — read
// the same as they always did.
func NewTestable(
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	w wizardPkg.Wizard,
	asm assembler.Assembler,
	markInstalled MarkInstalledFn,
	markUninstalled MarkUninstalledFn,
	markLastUsed MarkLastUsedFn,
	hasDependents HasDependentsFn,
	listArrows ListArrowsFn,
	reconcileVersionBadge ...func(ctx context.Context, ns domain.Namespace) error,
) (Runtime, error) {
	repo := &runtimeRepository{
		axRuntime:     axRuntime,
		wizard:        w,
		assembler:     asm,
		hasDependents: hasDependents,
		listArrows:    listArrows,
	}

	hooks := runtimeinternal.CatalogHooks{
		MarkInstalled:   markInstalled,
		MarkUninstalled: markUninstalled,
		MarkLastUsed:    markLastUsed,
	}
	if len(reconcileVersionBadge) > 0 {
		hooks.ReconcileVersionBadge = reconcileVersionBadge[0]
	}

	if err := runtimeinternal.RegisterReactions(
		axRuntime, hooks, w, repo.tryAddDrain,
	); err != nil {
		return nil, fmt.Errorf("runtime: register reactions: %w", err)
	}

	return repo, nil
}

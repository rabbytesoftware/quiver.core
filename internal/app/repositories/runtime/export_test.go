package runtime

import (
	"fmt"

	"github.com/char2cs/asynx"
	runtimeinternal "github.com/rabbytesoftware/quiver/internal/app/repositories/runtime/internal"
	"github.com/rabbytesoftware/quiver/internal/app/repositories/runtime/internal/assembler"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	wizardPkg "github.com/rabbytesoftware/quiver/internal/engine/wizard"
)

// Re-export for tests.
type ResolvedExecution = assembler.ResolvedExecution

// NewTestable creates a Runtime with an injected Assembler, for unit testing.
func NewTestable(
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	w wizardPkg.Wizard,
	asm assembler.Assembler,
	markInstalled MarkInstalledFn,
	hasDependents HasDependentsFn,
	listArrows ListArrowsFn,
) (Runtime, error) {
	repo := &runtimeRepository{
		axRuntime:     axRuntime,
		wizard:        w,
		assembler:     asm,
		hasDependents: hasDependents,
		listArrows:    listArrows,
	}

	if err := runtimeinternal.RegisterReactions(axRuntime, markInstalled, w, &repo.drainWg); err != nil {
		return nil, fmt.Errorf("runtime: register reactions: %w", err)
	}

	return repo, nil
}

package deps

import (
	"context"
	"errors"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	arrowcmds "github.com/rabbytesoftware/quiver/internal/app/arrow/internal/commands"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/execution/runner"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	domainstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/deptree"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	wizstep "github.com/rabbytesoftware/quiver/internal/engine/wizard/step"
)

// DependenciesHandler handles DependenciesStep execution inside the wizard.
// It resolves and installs transitive dependencies for a namespace.
type DependenciesHandler interface {
	wizstep.Handler[domainstep.DependenciesStep]
}

type handler struct {
	depTree      deptree.DepTree
	vault        vault.Vault
	manifold     manifold.Manifold
	asynxArrow   asynx.Asynx[domain.Arrow]
	asynxRuntime asynx.Asynx[domainRuntime.ArrowRuntime]
	runner       runner.Runner
}

// New constructs a DependenciesHandler with a runner for synchronous installs.
func New(
	depTree deptree.DepTree,
	v vault.Vault,
	m manifold.Manifold,
	axArrow asynx.Asynx[domain.Arrow],
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	r runner.Runner,
) DependenciesHandler {
	return &handler{
		depTree:      depTree,
		vault:        v,
		manifold:     m,
		asynxArrow:   axArrow,
		asynxRuntime: axRuntime,
		runner:       r,
	}
}

func (h *handler) Execute(
	ctx context.Context,
	req wizstep.Request,
	_ domainstep.DependenciesStep,
) error {
	ns := domain.Namespace(req.NSKey)

	resolver := deptree.ResolverFunc(func(
		rCtx context.Context,
		depNs domain.Namespace,
	) ([]domain.Namespace, error) {
		manifest, err := h.resolveManifest(rCtx, depNs)
		if err != nil {
			return nil, err
		}

		return manifest.Dependencies, nil
	})

	orderedDeps, err := h.depTree.Resolve(ctx, ns, resolver)
	if err != nil {
		return err
	}

	var installed []domain.Namespace

	for _, dep := range orderedDeps {
		if dep == ns {
			continue
		}

		rt, rtErr := h.asynxRuntime.Get(
			ctx,
			dep.String(),
		)
		if rtErr != nil && !errors.Is(rtErr, asynxModels.ErrNotFound) {
			rtErr = nil
		}
		if rtErr == nil && rt.Namespace != "" && rt.State != domain.ArrowStateAbsent {
			continue
		}

		manifest, mErr := h.resolveManifest(ctx, dep)
		if mErr != nil {
			h.rollback(ctx, installed)
			return mErr
		}

		existing, getErr := h.asynxArrow.Get(ctx, dep.String())
		if errors.Is(getErr, asynxModels.ErrNotFound) ||
			existing.Namespace == "" {
			_, _ = h.asynxArrow.Send(ctx, arrowcmds.AddArrow{
				Namespace: dep,
				Manifest:  *manifest,
			})
		}

		if installErr := h.runner.ExecuteSync(
			ctx,
			dep,
			"_install",
			nil,
		); installErr != nil {
			h.rollback(ctx, installed)
			return installErr
		}

		installed = append(installed, dep)
	}

	h.updateIndirectDeps(ctx, ns, orderedDeps)

	return nil
}

func (h *handler) resolveManifest(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.ArrowManifest, error) {
	entry, _, err := h.vault.GetArrow(ctx, ns)

	if err == nil {
		return entry.Manifest, nil
	}

	if errors.Is(err, vault.ErrStale) {
		manifest, manifoldErr := h.manifold.ResolveArrow(ctx, ns)
		if manifoldErr != nil {
			return entry.Manifest, nil
		}

		_, putErr := h.vault.PutArrow(ctx, ns, manifest, nil)
		if putErr != nil {
			return nil, putErr
		}

		return manifest, nil
	}

	if errors.Is(err, vault.ErrNotCached) {
		manifest, manifoldErr := h.manifold.ResolveArrow(ctx, ns)
		if manifoldErr != nil {
			return nil, manifoldErr
		}

		_, putErr := h.vault.PutArrow(ctx, ns, manifest, nil)
		if putErr != nil {
			return nil, putErr
		}

		return manifest, nil
	}

	return nil, err
}

func (h *handler) rollback(
	ctx context.Context,
	installed []domain.Namespace,
) {
	for i := len(installed) - 1; i >= 0; i-- {
		dep := installed[i]

		rt, err := h.asynxRuntime.Get(ctx, dep.String())
		if err != nil || rt.State != domain.ArrowStateReady {
			continue
		}

		_ = h.runner.ExecuteSync(ctx, dep, "_uninstall", nil)
	}
}

func (h *handler) updateIndirectDeps(
	ctx context.Context,
	ns domain.Namespace,
	deptreeResult []domain.Namespace,
) {
	arrow, err := h.asynxArrow.Get(ctx, ns.String())
	if err != nil {
		return
	}

	directSet := make(map[string]bool)
	for _, dep := range arrow.Manifest.Dependencies {
		directSet[dep.String()] = true
	}

	var indirect []domain.Namespace
	for _, dep := range deptreeResult {
		if dep == ns || directSet[dep.String()] {
			continue
		}

		indirect = append(indirect, dep)
	}

	_, _ = h.vault.PutArrow(ctx, ns, &arrow.Manifest, indirect)
}

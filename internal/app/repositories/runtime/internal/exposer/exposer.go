package exposer

import (
	"context"
	"log/slog"

	exposerinternal "github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal/exposer/internal"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
)

type GetArrowFn = exposerinternal.GetArrowFn

type ListArrowsFn = exposerinternal.ListArrowsFn

type Exposer interface {
	Apply(
		ctx context.Context,
		ns domain.Namespace,
		workdir string,
	) *domainRuntime.ExposeResult
	Reapply(
		ctx context.Context,
		ns domain.Namespace,
	) *domainRuntime.ExposeResult
	ApplyVersioned(
		ctx context.Context,
		ns domain.Namespace,
	) *domainRuntime.ExposeResult
	Remove(
		ctx context.Context,
		ns domain.Namespace,
	)
}

type exposerService struct {
	shelf      shelf.Shelf
	vault      vault.Vault
	getArrow   GetArrowFn
	listArrows ListArrowsFn
	os         domain.OS
}

func New(
	s shelf.Shelf,
	v vault.Vault,
	getArrow GetArrowFn,
	listArrows ListArrowsFn,
	os domain.OS,
) Exposer {
	return &exposerService{
		shelf:      s,
		vault:      v,
		getArrow:   getArrow,
		listArrows: listArrows,
		os:         os,
	}
}

func WithExposed(
	lastReturn *domainRuntime.Return,
	exposed *domainRuntime.ExposeResult,
) *domainRuntime.Return {
	if lastReturn == nil {
		return nil
	}
	copied := *lastReturn
	copied.Exposed = exposed
	return &copied
}

func (e *exposerService) Apply(
	ctx context.Context,
	ns domain.Namespace,
	workdir string,
) *domainRuntime.ExposeResult {
	if e.shelf == nil {
		return nil
	}
	arrow, ok := e.arrow(ctx, ns)
	if !ok {
		return nil
	}
	return e.apply(ctx, ns, workdir, arrow)
}

func (e *exposerService) Reapply(
	ctx context.Context,
	ns domain.Namespace,
) *domainRuntime.ExposeResult {
	if e.shelf == nil {
		return nil
	}
	arrow, ok := e.arrow(ctx, ns)
	if !ok || arrow.Targets[e.os].Expose.IsEmpty() {
		return nil
	}
	return e.applyAtWorkdir(ctx, ns, arrow)
}

func (e *exposerService) ApplyVersioned(
	ctx context.Context,
	ns domain.Namespace,
) *domainRuntime.ExposeResult {
	if e.shelf == nil {
		return nil
	}
	arrow, ok := e.arrow(ctx, ns)
	if !ok {
		return nil
	}
	return e.applyAtWorkdir(ctx, ns, arrow)
}

func (e *exposerService) Remove(
	ctx context.Context,
	ns domain.Namespace,
) {
	if e.shelf == nil {
		return
	}
	kept, err := exposerinternal.Installed(ctx, ns, e.getArrow, e.listArrows)
	if err != nil {
		slog.WarnContext(ctx, "runtime: unexpose: list catalog", "ns", ns, "err", err)
		return
	}
	if kept {
		return
	}
	if err := e.shelf.Remove(ctx, ns); err != nil {
		slog.WarnContext(ctx, "runtime: unexpose", "ns", ns, "err", err)
	}
}

func (e *exposerService) applyAtWorkdir(
	ctx context.Context,
	ns domain.Namespace,
	arrow *domain.Arrow,
) *domainRuntime.ExposeResult {
	workdir, ok := e.workdir(ctx, ns)
	if !ok {
		return nil
	}
	return e.apply(ctx, ns, workdir, arrow)
}

func (e *exposerService) apply(
	ctx context.Context,
	ns domain.Namespace,
	workdir string,
	arrow *domain.Arrow,
) *domainRuntime.ExposeResult {
	applied, err := e.shelf.Apply(ctx, ns, workdir, arrow.Targets[e.os].Expose, arrow.Media)
	if err != nil {
		slog.WarnContext(ctx, "runtime: expose", "ns", ns, "workdir", workdir, "err", err)
	}
	return exposerinternal.From(applied)
}

func (e *exposerService) arrow(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, bool) {
	arrow, err := e.getArrow(ctx, ns)
	if err != nil {
		slog.WarnContext(ctx, "runtime: expose: get arrow", "ns", ns, "err", err)
		return nil, false
	}
	return arrow, arrow != nil
}

func (e *exposerService) workdir(
	ctx context.Context,
	ns domain.Namespace,
) (string, bool) {
	if e.vault == nil {
		return "", false
	}
	workdir, err := e.vault.WorkDir(ctx, ns)
	if err != nil {
		slog.WarnContext(ctx, "runtime: expose: workdir", "ns", ns, "err", err)
		return "", false
	}
	return workdir, true
}

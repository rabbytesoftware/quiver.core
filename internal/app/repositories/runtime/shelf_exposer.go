package runtime

import (
	"context"
	"errors"
	"log/slog"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
)

type shelfExposer struct {
	shelf      shelf.Shelf
	vault      vault.Vault
	getArrow   GetArrowFn
	listArrows ListArrowsFn
	os         domain.OS
}

func newShelfExposer(
	s shelf.Shelf,
	v vault.Vault,
	getArrow GetArrowFn,
	listArrows ListArrowsFn,
	os domain.OS,
) *shelfExposer {
	return &shelfExposer{
		shelf:      s,
		vault:      v,
		getArrow:   getArrow,
		listArrows: listArrows,
		os:         os,
	}
}

func (e *shelfExposer) Apply(
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

func (e *shelfExposer) Reapply(
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
	workdir, ok := e.workdir(ctx, ns)
	if !ok {
		return nil
	}
	return e.apply(ctx, ns, workdir, arrow)
}

func (e *shelfExposer) Remove(
	ctx context.Context,
	ns domain.Namespace,
) {
	if e.shelf == nil {
		return
	}
	kept, err := e.siblingInstalled(ctx, ns)
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

func (e *shelfExposer) applyVersioned(
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
	workdir, ok := e.workdir(ctx, ns)
	if !ok {
		return nil
	}
	return e.apply(ctx, ns, workdir, arrow)
}

func (e *shelfExposer) apply(
	ctx context.Context,
	ns domain.Namespace,
	workdir string,
	arrow *domain.Arrow,
) *domainRuntime.ExposeResult {
	applied, err := e.shelf.Apply(ctx, ns, workdir, arrow.Targets[e.os].Expose, arrow.Media)
	if err != nil {
		slog.WarnContext(ctx, "runtime: expose", "ns", ns, "workdir", workdir, "err", err)
	}
	return exposeResultFrom(applied)
}

func (e *shelfExposer) arrow(
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

func (e *shelfExposer) workdir(
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

func (e *shelfExposer) siblingInstalled(
	ctx context.Context,
	ns domain.Namespace,
) (bool, error) {
	views, err := e.listArrows(ctx)
	if err != nil {
		return false, err
	}
	for _, view := range views {
		if e.viewHasInstalledSibling(ctx, view, ns) {
			return true, nil
		}
	}
	return false, nil
}

func (e *shelfExposer) viewHasInstalledSibling(
	ctx context.Context,
	view models.ArrowView,
	ns domain.Namespace,
) bool {
	for _, ver := range view.Versions {
		if ver.Namespace == ns || ver.Namespace.BareNamespace() != ns.BareNamespace() {
			continue
		}
		if e.installed(ctx, ver.Namespace) {
			return true
		}
	}
	return false
}

func (e *shelfExposer) installed(
	ctx context.Context,
	ns domain.Namespace,
) bool {
	arrow, err := e.getArrow(ctx, ns)
	if errors.Is(err, asynxModels.ErrNotFound) {
		return false
	}
	if err != nil {
		return true
	}
	return arrow != nil && !arrow.InstalledAt.IsZero()
}

func exposeResultFrom(
	applied shelf.Applied,
) *domainRuntime.ExposeResult {
	if len(applied.Entries) == 0 && len(applied.Refused) == 0 {
		return nil
	}
	result := &domainRuntime.ExposeResult{}
	for _, entry := range applied.Entries {
		result.Entries = append(result.Entries, domainRuntime.ExposedEntry{
			Kind:     entry.Kind,
			Name:     entry.Name,
			Target:   entry.Target,
			Location: entry.Location,
		})
	}
	for _, refusal := range applied.Refused {
		result.Refused = append(result.Refused, domainRuntime.ExposeRefusal{
			Kind:   refusal.Kind,
			Name:   refusal.Name,
			Reason: refusal.Reason,
		})
	}
	return result
}

func withExposed(
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

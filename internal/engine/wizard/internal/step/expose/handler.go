package expose

import (
	"context"
	"errors"
	"fmt"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainstep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf"
	wizstep "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step"
)

type Handler interface {
	Expose(
		ctx context.Context,
		req wizstep.Request,
		steps []domainstep.ExposeStep,
	) []error
	Execute(
		ctx context.Context,
		req wizstep.Request,
		s domainstep.UnexposeStep,
	) error
}

type handler struct {
	shelf shelf.Shelf
}

func NewHandler(
	s shelf.Shelf,
) Handler {
	return &handler{shelf: s}
}

type slot struct {
	kind  domain.ExposeKind
	index int
}

// Expose applies every step in one shelf pass, since a desktop bundle moved out
// of the workdir relocates the CLI entries pointing into it and the pass prunes
// what it did not place; each step then reports its own entry's outcome.
func (h *handler) Expose(
	ctx context.Context,
	req wizstep.Request,
	steps []domainstep.ExposeStep,
) []error {
	errs := make([]error, len(steps))
	block, slots, media := collect(steps, errs)

	applied, err := h.shelf.Apply(ctx, domain.Namespace(req.NSKey), req.WorkDir, block, media)
	for k, sl := range slots {
		if errs[k] != nil {
			continue
		}
		errs[k] = outcome(applied, sl, err)
	}
	return errs
}

func (h *handler) Execute(
	ctx context.Context,
	req wizstep.Request,
	_ domainstep.UnexposeStep,
) error {
	return h.shelf.Remove(ctx, req.WorkDir)
}

func collect(
	steps []domainstep.ExposeStep,
	errs []error,
) (domain.Expose, []slot, domain.ArrowMedia) {
	var block domain.Expose
	var media domain.ArrowMedia
	slots := make([]slot, len(steps))
	for k, s := range steps {
		media.Icon = s.MediaIcon
		entry := domain.ExposeEntry{Name: s.Name, Path: s.Path, Icon: s.Icon, Categories: s.Categories}
		switch kind := domain.ExposeKind(s.Kind); kind {
		case domain.ExposeKindDesktop:
			slots[k] = slot{kind: kind, index: len(block.Desktop)}
			block.Desktop = append(block.Desktop, entry)
		case domain.ExposeKindCLI:
			slots[k] = slot{kind: kind, index: len(block.CLI)}
			block.CLI = append(block.CLI, entry)
		default:
			errs[k] = fmt.Errorf("%s %s: %w", s.Kind, s.Name, ErrUnknownKind)
		}
	}
	return block, slots, media
}

func outcome(
	applied shelf.Applied,
	sl slot,
	applyErr error,
) error {
	var refusals []error
	for _, r := range applied.Refused {
		if r.Kind == sl.kind && r.Index == sl.index {
			refusals = append(refusals, fmt.Errorf("%s %s: %s: %w", r.Kind, r.Name, r.Reason, ErrRefused))
		}
	}
	if len(refusals) > 0 {
		return errors.Join(refusals...)
	}
	if applyErr == nil || placed(applied, sl) {
		return nil
	}
	return applyErr
}

func placed(
	applied shelf.Applied,
	sl slot,
) bool {
	for _, e := range applied.Entries {
		if e.Kind == sl.kind && e.Index == sl.index {
			return true
		}
	}
	return false
}

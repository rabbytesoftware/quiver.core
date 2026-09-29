package shelf

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/cli"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/desktop"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/ownership"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/pathenv"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/platform"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/resolve"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/workfs"
)

type (
	Applied      = models.Applied
	AppliedEntry = models.AppliedEntry
	Refusal      = models.Refusal
	PathStatus   = pathenv.PathStatus
)

type Shelf interface {
	Apply(
		ctx context.Context,
		ns domain.Namespace,
		workdir string,
		expose domain.Expose,
		media domain.ArrowMedia,
	) (Applied, error)
	Remove(
		ctx context.Context,
		workdir string,
	) error
	PathStatus(
		ctx context.Context,
	) (PathStatus, error)
	SetupPath(
		ctx context.Context,
	) (PathStatus, error)
}

type shelf struct {
	host     platform.Host
	userPath pathenv.UserPath
	resolver resolve.Resolver
	cli      cli.Placer
	desktop  desktop.Placer
}

type Option func(*platform.Host)

func New(
	opts ...Option,
) Shelf {
	host := platform.NewHost()
	for _, opt := range opts {
		opt(&host)
	}
	return newShelf(host, ownership.NewTagger(), pathenv.NewUserPath())
}

func WithSandboxHome(
	home string,
) Option {
	return func(h *platform.Host) {
		h.HomeDir = home
		h.UserHomeDir = home
		h.SandboxHome = home
		h.AppsDirs = []string{filepath.Join(home, "Applications")}
	}
}

func newShelf(
	host platform.Host,
	tagger ownership.Tagger,
	userPath pathenv.UserPath,
) *shelf {
	bundles := ownership.NewBundles(tagger)
	return &shelf{
		host:     host,
		userPath: userPath,
		resolver: resolve.New(host, bundles),
		cli:      cli.New(host, bundles),
		desktop:  desktop.New(host, bundles),
	}
}

func (s *shelf) Apply(
	ctx context.Context,
	ns domain.Namespace,
	workdir string,
	expose domain.Expose,
	media domain.ArrowMedia,
) (Applied, error) {
	req, err := s.request(ns, workdir, media)
	if err != nil {
		return Applied{}, fmt.Errorf("shelf: apply %s: %w", ns, err)
	}

	out := Applied{}
	if err := s.applyAll(ctx, req, domain.ExposeKindDesktop, expose.Desktop, &out); err != nil {
		return out, err
	}
	if err := s.applyAll(ctx, req, domain.ExposeKindCLI, expose.CLI, &out); err != nil {
		return out, err
	}
	if err := s.prune(ctx, req.Layout, ownership.NamespaceClaim(req.Bare), out.Locations()); err != nil {
		return out, fmt.Errorf("shelf: apply %s: prune: %w", ns, err)
	}
	return out, nil
}

func (s *shelf) Remove(
	ctx context.Context,
	workdir string,
) error {
	l, err := s.host.Layout()
	if err != nil {
		return fmt.Errorf("shelf: remove %s: %w", workdir, err)
	}

	clean := filepath.Clean(workdir)
	bare := ownership.WorkdirOwner(l.Namespaces, clean)
	if bare == "" {
		return fmt.Errorf("shelf: remove %s: %w", workdir, ErrNotAWorkdir)
	}

	if err := s.prune(ctx, l, ownership.WorkdirClaim(bare, clean), nil); err != nil {
		return fmt.Errorf("shelf: remove %s: %w", workdir, err)
	}
	return nil
}

func (s *shelf) PathStatus(
	_ context.Context,
) (PathStatus, error) {
	return pathenv.New(s.host, s.userPath).Status()
}

func (s *shelf) SetupPath(
	_ context.Context,
) (PathStatus, error) {
	return pathenv.New(s.host, s.userPath).Setup()
}

func (s *shelf) request(
	ns domain.Namespace,
	workdir string,
	media domain.ArrowMedia,
) (models.ApplyRequest, error) {
	if err := ns.Validate(); err != nil {
		return models.ApplyRequest{}, err
	}

	l, err := s.host.Layout()
	if err != nil {
		return models.ApplyRequest{}, err
	}

	bare := ns.BareNamespace()
	clean := filepath.Clean(workdir)
	if ownership.WorkdirOwner(l.Namespaces, clean) != bare {
		return models.ApplyRequest{}, fmt.Errorf("%s of %s: %w", workdir, bare, ErrNotAWorkdir)
	}

	return models.ApplyRequest{
		Layout:  l,
		Bare:    bare,
		Workdir: clean,
		Media:   media,
		Moved:   map[string]string{},
	}, nil
}

func (s *shelf) applyAll(
	ctx context.Context,
	req models.ApplyRequest,
	kind domain.ExposeKind,
	entries []domain.ExposeEntry,
	out *Applied,
) error {
	for i, entry := range entries {
		if err := s.applyEntry(ctx, req, kind, i, entry, out); err != nil {
			return err
		}
	}
	return nil
}

func (s *shelf) applyEntry(
	ctx context.Context,
	req models.ApplyRequest,
	kind domain.ExposeKind,
	index int,
	entry domain.ExposeEntry,
	out *Applied,
) error {
	candidates, reason, err := s.resolver.Resolve(req, kind, entry)
	if err != nil {
		return fmt.Errorf("shelf: apply %s %s: %w", kind, entry.Name, err)
	}
	if autoAbsent(entry, reason) {
		return nil
	}
	if reason != "" {
		out.Refuse(kind, index, entry.Name, reason)
		return nil
	}

	for _, c := range candidates {
		c.Target = relocate(req.Moved, c.Target)
		p, err := s.place(ctx, req, kind, entry, c)
		if err != nil {
			return fmt.Errorf("shelf: apply %s %s: %w", kind, c.Name, err)
		}
		if autoAbsent(entry, p.Refused) {
			continue
		}
		out.Record(kind, index, c, p)
	}
	return nil
}

func relocate(
	moved map[string]string,
	target string,
) string {
	for src, dest := range moved {
		if workfs.Inside(src, target) {
			return workfs.Relocate(target, src, dest)
		}
	}
	return target
}

func autoAbsent(
	entry domain.ExposeEntry,
	reason string,
) bool {
	if entry.Path != domain.ExposeAuto {
		return false
	}
	return reason == models.ReasonNoExecutable || reason == models.ReasonNoDesktop || reason == models.ReasonNotFound
}

func (s *shelf) place(
	ctx context.Context,
	req models.ApplyRequest,
	kind domain.ExposeKind,
	entry domain.ExposeEntry,
	c models.Candidate,
) (models.Placement, error) {
	switch kind {
	case domain.ExposeKindCLI:
		return s.cli.Place(ctx, req, c)
	case domain.ExposeKindDesktop:
		return s.desktop.Place(ctx, req, entry, c)
	}
	return models.Placement{}, fmt.Errorf("unknown expose kind %q", kind)
}

func (s *shelf) prune(
	ctx context.Context,
	l platform.Layout,
	claim ownership.Claim,
	keep map[string]bool,
) error {
	cliErr := s.cli.Remove(l, claim, keep)
	desktopErr := s.desktop.Remove(ctx, l, claim, keep)
	return errors.Join(cliErr, desktopErr)
}

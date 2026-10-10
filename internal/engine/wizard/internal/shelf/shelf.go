package shelf

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"runtime"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/discover"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/host"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/launch"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/ownership"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/platform"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/userpath"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/workfs"
)

type (
	Applied      = models.Applied
	AppliedEntry = models.AppliedEntry
	Refusal      = models.Refusal
	PathStatus   = models.PathStatus
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
	// Launchable reports whether the arrow installed in workdir has a desktop
	// entry Launch can start.
	Launchable(
		ctx context.Context,
		workdir string,
	) bool
	// Launch starts the desktop entry the arrow installed in workdir exposed,
	// detached from the daemon.
	Launch(
		ctx context.Context,
		workdir string,
	) error
}

type shelf struct {
	host     host.Host
	platform platform.Platform
}

type config struct {
	host  host.Host
	seams platform.Seams
}

type Option func(*config)

func New(
	opts ...Option,
) Shelf {
	cfg := config{
		host:  host.New(),
		seams: platform.Seams{Tagger: ownership.NewTagger(), UserPath: userpath.New()},
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return newShelf(runtime.GOOS, cfg.host, cfg.seams)
}

func WithSandboxHome(
	home string,
) Option {
	return func(c *config) {
		c.host.HomeDir = home
		c.host.UserHomeDir = home
		c.host.SandboxHome = home
		c.host.AppsDirs = []string{filepath.Join(home, "Applications")}
		c.seams.UserPath = userpath.NewFile(filepath.Join(home, "user-path"))
	}
}

func newShelf(
	goos string,
	h host.Host,
	seams platform.Seams,
) *shelf {
	return &shelf{host: h, platform: platform.ForOS(goos, h, seams)}
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
	if err := s.prune(ctx, req.Layout, models.NamespaceClaim(req.Bare), out.Locations()); err != nil {
		return out, fmt.Errorf("shelf: apply %s: prune: %w", ns, err)
	}
	if err := s.recordLaunch(req.Workdir, out); err != nil {
		return out, fmt.Errorf("shelf: apply %s: record launch: %w", ns, err)
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

	if err := s.prune(ctx, l, models.WorkdirClaim(bare, clean), nil); err != nil {
		return fmt.Errorf("shelf: remove %s: %w", workdir, err)
	}
	if err := launch.Clear(clean); err != nil {
		return fmt.Errorf("shelf: remove %s: clear launch: %w", workdir, err)
	}
	return nil
}

func (s *shelf) PathStatus(
	ctx context.Context,
) (PathStatus, error) {
	return s.platform.Path.Status(ctx)
}

func (s *shelf) SetupPath(
	ctx context.Context,
) (PathStatus, error) {
	return s.platform.Path.Setup(ctx)
}

func (s *shelf) request(
	ns domain.Namespace,
	workdir string,
	media domain.ArrowMedia,
) (models.Request, error) {
	if err := ns.Validate(); err != nil {
		return models.Request{}, err
	}

	l, err := s.host.Layout()
	if err != nil {
		return models.Request{}, err
	}

	bare := ns.BareNamespace()
	clean := filepath.Clean(workdir)
	if ownership.WorkdirOwner(l.Namespaces, clean) != bare {
		return models.Request{}, fmt.Errorf("%s of %s: %w", workdir, bare, ErrNotAWorkdir)
	}

	return models.Request{
		Layout:  l,
		Bare:    bare,
		Workdir: clean,
		Media:   media,
		Moved:   map[string]string{},
	}, nil
}

func (s *shelf) applyAll(
	ctx context.Context,
	req models.Request,
	kind domain.ExposeKind,
	entries []domain.ExposeEntry,
	out *Applied,
) error {
	exposer, err := s.exposer(kind)
	if err != nil {
		return err
	}
	for i, entry := range entries {
		if err := s.applyEntry(ctx, req, exposer, kind, i, entry, out); err != nil {
			return err
		}
	}
	return nil
}

func (s *shelf) applyEntry(
	ctx context.Context,
	req models.Request,
	exposer models.Exposer,
	kind domain.ExposeKind,
	index int,
	entry domain.ExposeEntry,
	out *Applied,
) error {
	candidates, reason, err := find(req, exposer, entry)
	if err != nil {
		return fmt.Errorf("shelf: apply %s %s: %w", kind, entry.Name, err)
	}
	if autoAbsent(entry, reason) {
		skip(ctx, req, out, kind, index, entry.Name, reason)
		return nil
	}
	if reason != "" {
		out.Refuse(kind, index, entry.Name, reason)
		return nil
	}

	for _, c := range candidates {
		c.Target = relocate(req.Moved, c.Target)
		p, err := s.place(ctx, req, exposer, entry, c)
		if err != nil {
			return fmt.Errorf("shelf: apply %s %s: %w", kind, c.Name, err)
		}
		if autoAbsent(entry, p.Refused) {
			skip(ctx, req, out, kind, index, c.Name, p.Refused)
			continue
		}
		out.Record(kind, index, c, p)
	}
	return nil
}

func (s *shelf) exposer(
	kind domain.ExposeKind,
) (models.Exposer, error) {
	switch kind {
	case domain.ExposeKindCLI:
		return s.platform.CLI, nil
	case domain.ExposeKindDesktop:
		return s.platform.Desktop, nil
	}
	return nil, fmt.Errorf("unknown expose kind %q", kind)
}

func find(
	req models.Request,
	exposer models.Exposer,
	entry domain.ExposeEntry,
) ([]models.Candidate, string, error) {
	if entry.Path == domain.ExposeAuto {
		return exposer.Find(req, entry)
	}
	return discover.Declared(req, entry)
}

func (s *shelf) place(
	ctx context.Context,
	req models.Request,
	exposer models.Exposer,
	entry domain.ExposeEntry,
	c models.Candidate,
) (models.Placement, error) {
	if !s.platform.SafeName(c.Name) {
		return models.Placement{Refused: models.ReasonUnsafeName}, nil
	}
	return exposer.Place(ctx, req, entry, c)
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

func skip(
	ctx context.Context,
	req models.Request,
	out *Applied,
	kind domain.ExposeKind,
	index int,
	name string,
	reason string,
) {
	out.Skip(kind, index, name, reason)
	slog.InfoContext(ctx, "shelf: auto entry skipped, nothing exposed", "ns", req.Bare, "kind", kind, "name", name, "reason", reason)
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

func (s *shelf) prune(
	ctx context.Context,
	l models.Layout,
	claim models.Claim,
	keep map[string]bool,
) error {
	cliErr := s.platform.CLI.Remove(ctx, l, claim, keep)
	desktopErr := s.platform.Desktop.Remove(ctx, l, claim, keep)
	return errors.Join(cliErr, desktopErr)
}

package shelf

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/cli"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/desktop"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/fsguard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/ownership"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/pathenv"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/platform"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/resolve"
)

type (
	Applied      = models.Applied
	AppliedEntry = models.AppliedEntry
	Refusal      = models.Refusal
	PathStatus   = pathenv.PathStatus
	Commander    = platform.Commander
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
		ns domain.Namespace,
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
	tagger   ownership.Tagger
	userPath pathenv.UserPath
}

type Option func(*shelf)

func New(
	opts ...Option,
) Shelf {
	s := &shelf{
		host:     platform.NewHost(),
		tagger:   ownership.NewTagger(),
		userPath: pathenv.NewUserPath(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func WithHomeDir(
	dir string,
) Option {
	return func(s *shelf) { s.host.HomeDir = dir }
}

func WithUserHomeDir(
	dir string,
) Option {
	return func(s *shelf) { s.host.UserHomeDir = dir }
}

func WithAppsDirs(
	dirs []string,
) Option {
	return func(s *shelf) { s.host.AppsDirs = dirs }
}

func WithGOOS(
	goos string,
) Option {
	return func(s *shelf) { s.host.GOOS = goos }
}

func WithGOARCH(
	goarch string,
) Option {
	return func(s *shelf) { s.host.GOARCH = goarch }
}

func WithCommander(
	c Commander,
) Option {
	return func(s *shelf) { s.host.Commander = c }
}

func WithEnv(
	lookup func(string) string,
) Option {
	return func(s *shelf) { s.host.Env = lookup }
}

func withTagger(
	t ownership.Tagger,
) Option {
	return func(s *shelf) { s.tagger = t }
}

func withUserPath(
	u pathenv.UserPath,
) Option {
	return func(s *shelf) { s.userPath = u }
}

func WithSandboxHome(
	home string,
) Option {
	return func(s *shelf) {
		s.host.HomeDir = home
		s.host.UserHomeDir = home
		s.host.SandboxHome = home
		s.host.AppsDirs = []string{filepath.Join(home, "Applications")}
	}
}

func (s *shelf) bundles() ownership.Bundles {
	return ownership.NewBundles(s.tagger)
}

func (s *shelf) resolver() resolve.Resolver {
	return resolve.New(s.host, s.bundles())
}

func (s *shelf) cliPlacer() cli.Placer {
	return cli.New(s.host, s.bundles())
}

func (s *shelf) desktopPlacer() desktop.Placer {
	return desktop.New(s.host, s.bundles())
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
	if err := s.prune(ctx, req.Layout, req.Bare, out.Locations()); err != nil {
		return out, fmt.Errorf("shelf: apply %s: prune: %w", ns, err)
	}
	return out, nil
}

func (s *shelf) Remove(
	ctx context.Context,
	ns domain.Namespace,
) error {
	if err := ns.Validate(); err != nil {
		return fmt.Errorf("shelf: remove %s: %w", ns, err)
	}

	l, err := s.host.Layout()
	if err != nil {
		return fmt.Errorf("shelf: remove %s: %w", ns, err)
	}

	if err := s.prune(ctx, l, ns.BareNamespace(), nil); err != nil {
		return fmt.Errorf("shelf: remove %s: %w", ns, err)
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
		return models.ApplyRequest{}, fmt.Errorf("%s is not a workdir of %s", workdir, bare)
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
	for _, entry := range entries {
		if err := s.applyEntry(ctx, req, kind, entry, out); err != nil {
			return err
		}
	}
	return nil
}

func (s *shelf) applyEntry(
	ctx context.Context,
	req models.ApplyRequest,
	kind domain.ExposeKind,
	entry domain.ExposeEntry,
	out *Applied,
) error {
	candidates, reason, err := s.resolver().Resolve(req, kind, entry)
	if err != nil {
		return fmt.Errorf("shelf: apply %s %s: %w", kind, entry.Name, err)
	}
	if autoAbsent(entry, reason) {
		return nil
	}
	if reason != "" {
		out.Refuse(kind, entry.Name, reason)
		return nil
	}

	for _, c := range candidates {
		c.Target = fsguard.Relocate(req, c.Target)
		p, err := s.place(ctx, req, kind, entry, c)
		if err != nil {
			return fmt.Errorf("shelf: apply %s %s: %w", kind, c.Name, err)
		}
		if autoAbsent(entry, p.Refused) {
			continue
		}
		out.Record(kind, c, p)
	}
	return nil
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
		return s.cliPlacer().Place(ctx, req, c)
	case domain.ExposeKindDesktop:
		return s.desktopPlacer().Place(ctx, req, entry, c)
	}
	return models.Placement{}, fmt.Errorf("unknown expose kind %q", kind)
}

func (s *shelf) prune(
	ctx context.Context,
	l platform.Layout,
	bare domain.Namespace,
	keep map[string]bool,
) error {
	cliErr := s.cliPlacer().Remove(l, bare, keep)
	desktopErr := s.desktopPlacer().Remove(ctx, l, bare, keep)
	return errors.Join(cliErr, desktopErr)
}

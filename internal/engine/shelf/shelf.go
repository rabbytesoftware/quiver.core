package shelf

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

const (
	goosDarwin  = "darwin"
	goosWindows = "windows"
	goarchARM64 = "arm64"
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
	homeDir     string
	userHomeDir string
	sandboxHome string
	appsDirs    []string
	goos        string
	goarch      string
	commander   Commander
	env         func(string) string
	tagger      bundleTagger
	userPath    userPath
	rename      func(string, string) error
	chmod       func(string, os.FileMode) error
}

func New(
	opts ...Option,
) Shelf {
	s := &shelf{
		goos:      runtime.GOOS,
		goarch:    runtime.GOARCH,
		commander: defaultCommander,
		env:       os.Getenv,
		tagger:    defaultTagger,
		userPath:  defaultUserPath,
		rename:    os.Rename,
		chmod:     os.Chmod,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
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
	if err := s.prune(ctx, req.layout, req.bare, out.locations()); err != nil {
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

	l, err := s.layout()
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
	return s.pathStatus()
}

func (s *shelf) SetupPath(
	_ context.Context,
) (PathStatus, error) {
	return s.setupPath()
}

func (s *shelf) request(
	ns domain.Namespace,
	workdir string,
	media domain.ArrowMedia,
) (applyRequest, error) {
	if err := ns.Validate(); err != nil {
		return applyRequest{}, err
	}

	l, err := s.layout()
	if err != nil {
		return applyRequest{}, err
	}

	bare := ns.BareNamespace()
	clean := filepath.Clean(workdir)
	if workdirOwner(l.namespaces, clean) != bare {
		return applyRequest{}, fmt.Errorf("%s is not a workdir of %s", workdir, bare)
	}

	return applyRequest{
		layout:  l,
		bare:    bare,
		workdir: clean,
		media:   media,
		moved:   map[string]string{},
	}, nil
}

func (s *shelf) applyAll(
	ctx context.Context,
	req applyRequest,
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
	req applyRequest,
	kind domain.ExposeKind,
	entry domain.ExposeEntry,
	out *Applied,
) error {
	candidates, reason, err := s.resolve(req, kind, entry)
	if err != nil {
		return fmt.Errorf("shelf: apply %s %s: %w", kind, entry.Name, err)
	}
	if autoAbsent(entry, reason) {
		return nil
	}
	if reason != "" {
		out.refuse(kind, entry.Name, reason)
		return nil
	}

	for _, c := range candidates {
		c.target = req.relocate(c.target)
		p, err := s.place(ctx, req, kind, entry, c)
		if err != nil {
			return fmt.Errorf("shelf: apply %s %s: %w", kind, c.name, err)
		}
		if autoAbsent(entry, p.refused) {
			continue
		}
		out.record(kind, c, p)
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
	return reason == reasonNoExecutable || reason == reasonNoDesktop || reason == reasonNotFound
}

func (s *shelf) place(
	ctx context.Context,
	req applyRequest,
	kind domain.ExposeKind,
	entry domain.ExposeEntry,
	c candidate,
) (placement, error) {
	switch kind {
	case domain.ExposeKindCLI:
		return s.placeCLI(ctx, req, c)
	case domain.ExposeKindDesktop:
		return s.placeDesktop(ctx, req, entry, c)
	}
	return placement{}, fmt.Errorf("unknown expose kind %q", kind)
}

func (s *shelf) placeCLI(
	ctx context.Context,
	req applyRequest,
	c candidate,
) (placement, error) {
	if s.goos == goosWindows {
		return placeShim(req, c)
	}
	return s.placeSymlink(ctx, req, c)
}

func (s *shelf) placeDesktop(
	ctx context.Context,
	req applyRequest,
	entry domain.ExposeEntry,
	c candidate,
) (placement, error) {
	switch s.goos {
	case goosDarwin:
		return s.placeApp(ctx, req, c)
	case goosWindows:
		return s.placeLnk(ctx, req, entry, c)
	}
	return placeXDG(req, entry, c)
}

func (s *shelf) prune(
	ctx context.Context,
	l layout,
	bare domain.Namespace,
	keep map[string]bool,
) error {
	cliErr := s.removeCLI(l, bare, keep)
	desktopErr := s.removeDesktop(ctx, l, bare, keep)
	return errors.Join(cliErr, desktopErr)
}

func (s *shelf) removeCLI(
	l layout,
	bare domain.Namespace,
	keep map[string]bool,
) error {
	if s.goos == goosWindows {
		return removeShims(l, bare, keep)
	}
	return s.removeSymlinks(l, bare, keep)
}

func (s *shelf) removeDesktop(
	ctx context.Context,
	l layout,
	bare domain.Namespace,
	keep map[string]bool,
) error {
	switch s.goos {
	case goosDarwin:
		return s.removeApps(l, bare, keep)
	case goosWindows:
		return s.removeLnks(ctx, l, bare, keep)
	}
	return removeXDG(l, bare, keep)
}

package guard

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/core/fns"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/models"
)

const (
	copyChunk  = 1 << 20
	maxEntries = 1_000_000
	DirPerm    = 0o755
	ownerPerm  = 0o700
)

type Guard struct {
	root         *os.Root
	dest         string
	maxBytes     int64
	written      int64
	maxEntries   int
	entries      int
	links        []string
	tops         map[string]struct{}
	skipEscaping bool
}

type Option func(*Guard)

func SkipEscapingLinks() Option {
	return func(g *Guard) {
		g.skipEscaping = true
	}
}

func (g *Guard) tolerateEscape(
	err error,
) error {
	if g.skipEscaping && errors.Is(err, models.ErrEscape) {
		return nil
	}

	return err
}

func Open(
	ctx context.Context,
	dest string,
	maxBytes int64,
	opts ...Option,
) (*Guard, error) {
	if err := fns.MkdirAll(ctx, dest, DirPerm); err != nil {
		return nil, fmt.Errorf("unpack: create %s: %w", dest, err)
	}

	resolved, err := filepath.EvalSymlinks(dest)
	if err != nil {
		return nil, fmt.Errorf("unpack: resolve %s: %w", dest, err)
	}

	root, err := os.OpenRoot(resolved)
	if err != nil {
		return nil, fmt.Errorf("unpack: open destination %s: %w", dest, err)
	}

	g := &Guard{
		root:       root,
		dest:       resolved,
		maxBytes:   maxBytes,
		maxEntries: maxEntries,
		tops:       make(map[string]struct{}),
	}
	for _, opt := range opts {
		opt(g)
	}

	return g, nil
}

func (g *Guard) Close() {
	_ = g.root.Close()
}

// Verify re-checks every symlink once the whole tree exists: a target may
// point at an entry extracted after the link itself.
func (g *Guard) Verify() error {
	errs := make([]error, 0, len(g.links))
	for _, link := range g.links {
		errs = append(errs, g.verifyLink(link))
	}

	return errors.Join(errs...)
}

func (g *Guard) TopLevel() []string {
	names := make([]string, 0, len(g.tops))
	for name := range g.tops {
		names = append(names, name)
	}
	sort.Strings(names)

	return names
}

func (g *Guard) Dir(
	name string,
	perm os.FileMode,
) error {
	cleaned, err := cleanName(name)
	if err != nil {
		return err
	}
	if cleaned == "." {
		return nil
	}
	if err := g.admit(); err != nil {
		return err
	}

	if err := g.root.MkdirAll(cleaned, perm|ownerPerm); err != nil {
		return fmt.Errorf("unpack: mkdir %s: %w", cleaned, err)
	}
	g.recordTop(cleaned)

	return nil
}

func (g *Guard) File(
	ctx context.Context,
	name string,
	perm os.FileMode,
	r io.Reader,
) error {
	cleaned, err := entryName(name)
	if err != nil {
		return err
	}
	if err := g.admit(); err != nil {
		return err
	}

	if err := g.place(cleaned); err != nil {
		return err
	}

	f, err := g.root.OpenFile(cleaned, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("unpack: create %s: %w", cleaned, err)
	}

	if err := errors.Join(g.Copy(ctx, f, r), f.Close()); err != nil {
		_ = g.root.Remove(cleaned)
		return fmt.Errorf("unpack: write %s: %w", cleaned, err)
	}

	if err := g.root.Chmod(cleaned, perm); err != nil {
		return fmt.Errorf("unpack: chmod %s: %w", cleaned, err)
	}
	g.recordTop(cleaned)

	return nil
}

func (g *Guard) Symlink(
	name string,
	target string,
) error {
	cleaned, err := entryName(name)
	if err != nil {
		return err
	}
	if err := g.admit(); err != nil {
		return err
	}

	if err := checkLinkTarget(cleaned, target); err != nil {
		return g.tolerateEscape(err)
	}

	physical, err := g.physical(cleaned, target)
	if err != nil {
		return g.tolerateEscape(err)
	}

	if err := g.place(cleaned); err != nil {
		return err
	}

	if err := g.root.Symlink(target, cleaned); err != nil {
		return fmt.Errorf("unpack: symlink %s: %w", cleaned, err)
	}
	g.links = append(g.links, physical)
	g.recordTop(cleaned)

	return nil
}

func (g *Guard) Hardlink(
	name string,
	target string,
) error {
	cleaned, err := entryName(name)
	if err != nil {
		return err
	}
	if err := g.admit(); err != nil {
		return err
	}

	source, err := cleanName(target)
	if err != nil {
		return err
	}
	if source == "." {
		return fmt.Errorf("unpack: %s: invalid link target %q", cleaned, target)
	}

	if err := g.rejectSymlink(source); err != nil {
		return err
	}

	if err := g.place(cleaned); err != nil {
		return err
	}

	if err := g.root.Link(source, cleaned); err != nil {
		return fmt.Errorf("unpack: link %s: %w", cleaned, err)
	}
	g.recordTop(cleaned)

	return nil
}

func (g *Guard) admit() error {
	g.entries++
	if g.entries > g.maxEntries {
		return fmt.Errorf("limit %d entries: %w", g.maxEntries, models.ErrTooMany)
	}
	return nil
}

func (g *Guard) recordTop(
	cleaned string,
) {
	top := cleaned
	if idx := strings.IndexRune(cleaned, filepath.Separator); idx >= 0 {
		top = cleaned[:idx]
	}
	g.tops[top] = struct{}{}
}

func (g *Guard) verifyLink(
	link string,
) error {
	target, err := os.Readlink(link)
	if err != nil {
		return nil
	}

	_, _, err = newLinkResolver(g).resolve(filepath.Dir(link), target)
	if err == nil {
		return nil
	}

	rel, _ := filepath.Rel(g.dest, link)
	_ = g.root.Remove(rel)

	return g.tolerateEscape(fmt.Errorf("unpack: %s -> %s: %w", rel, target, err))
}

func (g *Guard) Copy(
	ctx context.Context,
	w io.Writer,
	r io.Reader,
) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		n, err := io.CopyN(w, r, min(copyChunk, g.maxBytes-g.written+1))
		g.written += n
		if g.written > g.maxBytes {
			return fmt.Errorf("limit %d bytes: %w", g.maxBytes, models.ErrTooLarge)
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func (g *Guard) place(
	cleaned string,
) error {
	_ = g.root.Remove(cleaned)

	parent := filepath.Dir(cleaned)
	if parent == "." {
		return nil
	}

	if err := g.root.MkdirAll(parent, DirPerm); err != nil {
		return fmt.Errorf("unpack: mkdir %s: %w", parent, err)
	}

	return nil
}

func (g *Guard) physical(
	cleaned string,
	target string,
) (string, error) {
	parent, err := resolveExisting(filepath.Join(g.dest, filepath.Dir(cleaned)))
	if err != nil {
		return "", fmt.Errorf("unpack: resolve %s: %w", cleaned, err)
	}

	physical := filepath.Join(parent, filepath.Base(cleaned))
	if !g.within(physical) || !g.within(filepath.Join(parent, filepath.FromSlash(target))) {
		return "", fmt.Errorf("unpack: %s -> %s: %w", cleaned, target, models.ErrEscape)
	}

	return physical, nil
}

func resolveExisting(
	path string,
) (string, error) {
	missing := ""
	for {
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil {
			return filepath.Join(resolved, missing), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}

		missing = filepath.Join(filepath.Base(path), missing)
		path = filepath.Dir(path)
	}
}

func (g *Guard) rejectSymlink(
	source string,
) error {
	info, err := g.root.Lstat(source)
	if err != nil {
		return fmt.Errorf("unpack: link %s: %w", source, err)
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("unpack: %s: hardlink to symlink is not allowed", source)
	}

	return nil
}

func (g *Guard) within(
	path string,
) bool {
	rel, err := filepath.Rel(g.dest, path)

	return err == nil && !escapes(rel)
}

func checkLinkTarget(
	cleaned string,
	target string,
) error {
	if target == "" {
		return fmt.Errorf("unpack: %s: invalid link target %q", cleaned, target)
	}

	if IsAbsolute(target) || escapes(filepath.Join(filepath.Dir(cleaned), filepath.FromSlash(target))) {
		return fmt.Errorf("unpack: %s -> %s: %w", cleaned, target, models.ErrEscape)
	}

	return nil
}

func entryName(
	name string,
) (string, error) {
	cleaned, err := cleanName(name)
	if err != nil {
		return "", err
	}

	if cleaned == "." {
		return "", fmt.Errorf("unpack: %q: invalid entry name", name)
	}

	return cleaned, nil
}

func cleanName(
	name string,
) (string, error) {
	if IsAbsolute(name) {
		return "", fmt.Errorf("unpack: %q: absolute path: %w", name, models.ErrEscape)
	}

	cleaned := filepath.Clean(filepath.FromSlash(name))
	if escapes(cleaned) {
		return "", fmt.Errorf("unpack: %q: %w", name, models.ErrEscape)
	}

	return cleaned, nil
}

func IsAbsolute(
	path string,
) bool {
	return strings.HasPrefix(path, "/") ||
		strings.HasPrefix(path, `\`) ||
		filepath.IsAbs(path) ||
		filepath.VolumeName(path) != ""
}

func escapes(
	rel string,
) bool {
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

package guard

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	rules        NameRules
	seen         map[string]string
}

type Option func(*Guard)

func SkipEscapingLinks() Option {
	return func(g *Guard) {
		g.skipEscaping = true
	}
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
		seen:       make(map[string]string),
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
	if err != nil || cleaned == "." {
		return err
	}
	if err := g.admit(cleaned); err != nil {
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
	cleaned, err := g.entry(name)
	if err != nil {
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

func (g *Guard) entry(
	name string,
) (string, error) {
	cleaned, err := entryName(name)
	if err != nil {
		return "", err
	}

	return cleaned, g.admit(cleaned)
}

func (g *Guard) admit(
	cleaned string,
) error {
	if err := g.admitName(cleaned); err != nil {
		return err
	}

	g.entries++
	if g.entries > g.maxEntries {
		return fmt.Errorf("limit %d entries: %w", g.maxEntries, models.ErrTooMany)
	}
	return nil
}

func (g *Guard) recordTop(
	cleaned string,
) {
	top, _, _ := strings.Cut(cleaned, string(filepath.Separator))
	g.tops[top] = struct{}{}
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

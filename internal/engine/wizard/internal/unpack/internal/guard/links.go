package guard

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/workfs"
)

const maxLinkHops = 40

func (g *Guard) Symlink(
	name string,
	target string,
) error {
	cleaned, err := g.entry(name)
	if err != nil {
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

	if err := g.root.Symlink(filepath.FromSlash(target), cleaned); err != nil {
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
	cleaned, err := g.entry(name)
	if err != nil {
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

func (g *Guard) tolerateEscape(
	err error,
) error {
	if g.skipEscaping && errors.Is(err, models.ErrEscape) {
		return nil
	}

	return err
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

func (g *Guard) physical(
	cleaned string,
	target string,
) (string, error) {
	parent, err := resolveExisting(filepath.Join(g.dest, filepath.Dir(cleaned)))
	if err != nil {
		return "", fmt.Errorf("unpack: resolve %s: %w", cleaned, err)
	}

	physical := filepath.Join(parent, filepath.Base(cleaned))
	if !workfs.Inside(g.dest, physical) || !workfs.Inside(g.dest, filepath.Join(parent, filepath.FromSlash(target))) {
		return "", fmt.Errorf("unpack: %s -> %s: %w", cleaned, target, models.ErrEscape)
	}

	return physical, nil
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

func checkLinkTarget(
	cleaned string,
	target string,
) error {
	if target == "" {
		return fmt.Errorf("unpack: %s: invalid link target %q", cleaned, target)
	}

	if IsAbsolute(target) || !workfs.RelInside(filepath.Join(filepath.Dir(cleaned), filepath.FromSlash(target))) {
		return fmt.Errorf("unpack: %s -> %s: %w", cleaned, target, models.ErrEscape)
	}

	return nil
}

type linkResolver struct {
	g    *Guard
	hops int
}

func newLinkResolver(
	g *Guard,
) *linkResolver {
	return &linkResolver{g: g, hops: maxLinkHops}
}

func (r *linkResolver) resolve(
	dir string,
	target string,
) (string, bool, error) {
	if IsAbsolute(target) {
		return "", false, fmt.Errorf("unpack: absolute link target %s: %w", target, models.ErrEscape)
	}

	parts := strings.Split(filepath.FromSlash(target), string(filepath.Separator))
	cur := dir
	for i, part := range parts {
		next, complete, err := r.walk(cur, part)
		if err != nil {
			return "", false, err
		}
		if !complete {
			return r.contain(filepath.Join(append([]string{next}, parts[i+1:]...)...), false)
		}
		cur = next
	}

	return cur, true, nil
}

func (r *linkResolver) walk(
	cur string,
	part string,
) (string, bool, error) {
	switch part {
	case "", ".":
		return cur, true, nil
	case "..":
		return r.contain(filepath.Dir(cur), true)
	}

	next := filepath.Join(cur, part)
	info, err := os.Lstat(next)
	if err != nil {
		return next, false, nil
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return next, true, nil
	}

	return r.follow(cur, next)
}

func (r *linkResolver) follow(
	cur string,
	link string,
) (string, bool, error) {
	r.hops--
	if r.hops < 0 {
		return "", false, fmt.Errorf("unpack: %s: too many levels of symbolic links: %w", link, models.ErrEscape)
	}

	target, err := os.Readlink(link)
	if err != nil {
		return "", false, fmt.Errorf("unpack: readlink %s: %w", link, err)
	}

	return r.resolve(cur, target)
}

func (r *linkResolver) contain(
	path string,
	complete bool,
) (string, bool, error) {
	if !workfs.Inside(r.g.dest, path) {
		return "", false, fmt.Errorf("unpack: %s: %w", path, models.ErrEscape)
	}

	return path, complete, nil
}

package guard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/models"
)

const maxLinkHops = 40

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
	if !r.g.within(path) {
		return "", false, fmt.Errorf("unpack: %s: %w", path, models.ErrEscape)
	}

	return path, complete, nil
}

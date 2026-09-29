package dmg

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/guard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/models"
)

const (
	trailerSize  = 512
	bundleSuffix = ".app"
)

type mounter struct {
	attach func(ctx context.Context, image, mount string) error
	detach func(ctx context.Context, mount string)
}

type dmg struct {
	src      *os.File
	maxBytes int64
	rules    guard.HostRules
	mounter  mounter
}

func New(
	maxBytes int64,
	rules guard.HostRules,
) models.Detect {
	return newWith(maxBytes, rules, hdiutil())
}

func newWith(
	maxBytes int64,
	rules guard.HostRules,
	m mounter,
) models.Detect {
	return func(src *os.File, size int64) (models.Format, bool, error) {
		if !is(src, size) {
			return nil, false, nil
		}

		return &dmg{src: src, maxBytes: maxBytes, rules: rules, mounter: m}, true, nil
	}
}

func (d *dmg) Kind() models.Kind {
	return models.KindDmg
}

func (d *dmg) Unit() models.Unit {
	return models.Unit{}
}

func (d *dmg) Unpack(
	ctx context.Context,
	target models.Target,
) (models.Result, error) {
	g, err := guard.Open(ctx, target.Dir, d.maxBytes, guard.WithHostRules(d.rules))
	if err != nil {
		return models.Result{}, err
	}
	defer g.Close()

	if err := errors.Join(d.copyImage(ctx, g), g.Verify()); err != nil {
		return models.Result{}, err
	}

	return models.Result{Apps: g.Apps(target.Dir, bundleSuffix, true)}, nil
}

func (d *dmg) copyImage(
	ctx context.Context,
	g *guard.Guard,
) error {
	mount, err := os.MkdirTemp("", "quiver-dmg-")
	if err != nil {
		return fmt.Errorf("unpack: dmg: mountpoint: %w", err)
	}
	defer os.Remove(mount) //nolint:errcheck

	if err := d.mounter.attach(ctx, d.src.Name(), mount); err != nil {
		return err
	}
	defer d.mounter.detach(ctx, mount)

	return g.CopyTree(ctx, mount, route)
}

// route drops the volume's own top-level decorations: hidden entries such as
// .background or .fseventsd, and the absolute /Applications drop-target link.
func route(
	path string,
	rel string,
	d fs.DirEntry,
) (string, bool) {
	if strings.ContainsAny(rel, `/\`) {
		return rel, true
	}
	if strings.HasPrefix(rel, ".") {
		return "", false
	}
	if d.Type()&fs.ModeSymlink == 0 {
		return rel, true
	}

	target, err := os.Readlink(path)

	return rel, err != nil || !guard.IsAbsolute(target)
}

func is(
	src io.ReaderAt,
	size int64,
) bool {
	trailer := make([]byte, 4)
	_, err := src.ReadAt(trailer, size-trailerSize)

	return err == nil && string(trailer) == "koly"
}

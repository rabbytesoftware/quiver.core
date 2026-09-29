package msi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/guard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/models"
)

const (
	oleMagic  = "\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1"
	msiSuffix = ".msi"
	exeSuffix = ".exe"
)

type msi struct {
	src      *os.File
	maxBytes int64
	rules    guard.NameRules
	run      runner
}

func New(
	maxBytes int64,
	rules guard.NameRules,
) models.Detect {
	return newWith(maxBytes, rules, msiexec)
}

func newWith(
	maxBytes int64,
	rules guard.NameRules,
	run runner,
) models.Detect {
	return func(src *os.File, _ int64) (models.Format, bool, error) {
		if !is(src) {
			return nil, false, nil
		}

		return &msi{src: src, maxBytes: maxBytes, rules: rules, run: run}, true, nil
	}
}

func (m *msi) Kind() models.Kind {
	return models.KindMsi
}

func (m *msi) Unit() models.Unit {
	return models.Unit{}
}

func (m *msi) Unpack(
	ctx context.Context,
	target models.Target,
) (models.Result, error) {
	g, err := guard.Open(ctx, target.Dir, m.maxBytes, guard.WithNameRules(m.rules))
	if err != nil {
		return models.Result{}, err
	}
	defer g.Close()

	if err := errors.Join(m.copyImage(ctx, g), g.Verify()); err != nil {
		return models.Result{}, err
	}

	return models.Result{Apps: g.Apps(target.Dir, exeSuffix, false)}, nil
}

func (m *msi) copyImage(
	ctx context.Context,
	g *guard.Guard,
) error {
	work, err := os.MkdirTemp("", "quiver-msi-")
	if err != nil {
		return fmt.Errorf("unpack: msi: workspace: %w", err)
	}
	defer os.RemoveAll(work) //nolint:errcheck

	image := filepath.Join(work, "image")
	if err := adminInstall(ctx, m.run, m.src.Name(), image, filepath.Join(work, "msiexec.log")); err != nil {
		return err
	}

	l, err := planLayout(image, filepath.Base(m.src.Name()))
	if err != nil {
		return err
	}

	return g.CopyTree(ctx, image, l.route)
}

func is(
	src *os.File,
) bool {
	if !strings.EqualFold(filepath.Ext(src.Name()), msiSuffix) {
		return false
	}

	head := make([]byte, len(oleMagic))
	_, err := io.ReadFull(io.NewSectionReader(src, 0, int64(len(oleMagic))), head)

	return err == nil && string(head) == oleMagic
}

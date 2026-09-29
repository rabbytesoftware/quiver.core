package install

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/portable/internal/dest"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/portable/internal/record"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack"
)

var ErrInvalidName = errors.New("portable: name must be a single file name")

type Request struct {
	NSKey   string
	WorkDir string
	From    string
	To      string
	Name    string
}

type Installer interface {
	Install(
		ctx context.Context,
		req Request,
	) error
}

type installer struct {
	unpacker unpack.Unpacker
}

func New(
	unpacker unpack.Unpacker,
) Installer {
	return &installer{unpacker: unpacker}
}

func (i *installer) Install(
	ctx context.Context,
	req Request,
) error {
	if err := validName(req.Name); err != nil {
		return err
	}

	target, err := dest.Claim(req.WorkDir, req.From, req.To)
	if err != nil {
		return err
	}

	result, err := i.place(ctx, target, req)
	if err != nil {
		return err
	}

	if err := record.Save(ctx, req.NSKey, req.WorkDir, result.Apps); err != nil {
		return err
	}

	return removeSource(req.WorkDir, req.From, result.Output)
}

func (i *installer) place(
	ctx context.Context,
	target dest.Destination,
	req Request,
) (unpack.Result, error) {
	src, err := os.Open(req.From) // #nosec G304 -- path is the step's own from: field, resolved against the workdir
	if err != nil {
		return unpack.Result{}, fmt.Errorf("portable: open %s: %w", req.From, err)
	}
	defer src.Close() //nolint:errcheck

	info, err := src.Stat()
	if err != nil {
		return unpack.Result{}, fmt.Errorf("portable: stat %s: %w", req.From, err)
	}

	format, err := i.unpacker.Detect(src, info.Size())
	if err != nil {
		return unpack.Result{}, err
	}

	return target.Stage(format.Unit(), func(dir string) (unpack.Result, error) {
		return format.Unpack(ctx, unpack.Target{Dir: dir, Name: req.Name})
	})
}

func validName(
	name string,
) error {
	if name == "" || (name != "." && filepath.Base(name) == name && filepath.IsLocal(name)) {
		return nil
	}

	return fmt.Errorf("%w: %q", ErrInvalidName, name)
}

func removeSource(
	workDir string,
	from string,
	output string,
) error {
	if from == output {
		return nil
	}

	if _, inside := dest.WorkdirRel(workDir, from); !inside {
		return nil
	}

	if err := os.Remove(from); err != nil {
		return fmt.Errorf("portable: remove source %s: %w", from, err)
	}

	return nil
}

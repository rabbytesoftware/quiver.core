package extract

import (
	"context"
	"errors"
	"fmt"
	"os"

	domainstep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	wizstep "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack"
)

var ErrPortableFormat = errors.New("portable app format, use type: portable")

type handler struct {
	unpacker unpack.Unpacker
}

func NewHandler(
	maxBytes int64,
) wizstep.Handler[domainstep.ExtractStep] {
	return &handler{unpacker: unpack.New(maxBytes)}
}

func (h *handler) Execute(
	ctx context.Context,
	req wizstep.Request,
	s domainstep.ExtractStep,
) error {
	osArch := req.OSArch.String()

	stepCtx, cancel, err := wizstep.WithTimeout(ctx, s.Timeout.Resolve(osArch))
	if err != nil {
		return fmt.Errorf("extract: %w", err)
	}
	defer cancel()

	from := req.ResolvePath(s.From.Resolve(osArch))
	to := req.ResolvePath(s.To.Resolve(osArch))

	src, err := os.Open(from) // #nosec G304 -- path is the step's own from: field, resolved against the workdir like every other file step
	if err != nil {
		return fmt.Errorf("extract: open %s: %w", from, err)
	}
	defer src.Close() //nolint:errcheck

	info, err := src.Stat()
	if err != nil {
		return fmt.Errorf("extract: stat %s: %w", from, err)
	}

	format, err := h.unpacker.Detect(src, info.Size())
	if err != nil {
		return err
	}
	if format.Kind() != unpack.KindArchive {
		return fmt.Errorf("extract: %s: %w", from, ErrPortableFormat)
	}

	_, err = format.Unpack(stepCtx, unpack.Target{Dir: to})

	return err
}

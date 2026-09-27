package shelf

import (
	"path/filepath"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

const (
	shimMarker    = "REM quiver:"
	shimExtension = ".cmd"
	shimReserved  = `"%!`
)

func shimContent(
	bare domain.Namespace,
	target string,
) string {
	return "@echo off\r\n" + shimMarker + string(bare) + "\r\n\"" + target + "\" %*\r\n"
}

func placeShim(
	req applyRequest,
	c candidate,
) (placement, error) {
	if unsafePath(c.target, shimReserved) {
		return placement{refused: reasonUnsafePath}, nil
	}

	reason, err := requireTarget(c.target, false)
	if err != nil || reason != "" {
		return placement{refused: reason}, err
	}

	loc := filepath.Join(req.layout.bin, c.name+shimExtension)
	h, err := fileHolder(loc, shimMarker)
	if err != nil {
		return placement{}, err
	}
	if refusal := h.refusal(req.bare); refusal != "" {
		return placement{refused: refusal}, nil
	}

	if err := swapFile(loc, []byte(shimContent(req.bare, c.target))); err != nil {
		return placement{}, err
	}
	return placement{location: loc}, nil
}

func removeShims(
	l layout,
	bare domain.Namespace,
	keep map[string]bool,
) error {
	return removeMarked(l.bin, "", shimExtension, shimMarker, bare, keep)
}

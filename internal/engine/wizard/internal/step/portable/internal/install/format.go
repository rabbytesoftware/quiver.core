package install

import (
	"bytes"
	"errors"
	"io"
	"os"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack"
)

type format int

const (
	formatUnknown format = iota
	formatAppImage
	formatDmg
	formatArchive
	formatExecutable
)

const executableMagicLen = 4

func detectFormat(
	src *os.File,
	size int64,
) (format, unpack.Archive, error) {
	if unpack.IsAppImage(src) {
		return formatAppImage, unpack.Archive{}, nil
	}

	if unpack.IsDmg(src, size) {
		return formatDmg, unpack.Archive{}, nil
	}

	archive, err := unpack.DetectArchive(src, size)
	if err == nil {
		return formatArchive, archive, nil
	}
	if !errors.Is(err, unpack.ErrUnknownFormat) {
		return formatUnknown, unpack.Archive{}, err
	}

	if isExecutable(src) {
		return formatExecutable, unpack.Archive{}, nil
	}

	return formatUnknown, unpack.Archive{}, nil
}

func isExecutable(
	src io.ReaderAt,
) bool {
	head := make([]byte, executableMagicLen)
	n, _ := src.ReadAt(head, 0)
	head = head[:n]

	for _, magic := range []string{"\x7fELF", "\xfe\xed\xfa\xcf", "\xcf\xfa\xed\xfe", "\xca\xfe\xba\xbe", "MZ"} {
		if bytes.HasPrefix(head, []byte(magic)) {
			return true
		}
	}

	return false
}

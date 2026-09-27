package unpack

import (
	"bytes"
	"io"
)

const appImageMagicLen = 11

func IsAppImage(
	src io.ReaderAt,
) bool {
	head := make([]byte, appImageMagicLen)
	if _, err := src.ReadAt(head, 0); err != nil {
		return false
	}

	if !bytes.HasPrefix(head, []byte("\x7fELF")) {
		return false
	}

	tag := string(head[8:appImageMagicLen])

	return tag == "AI\x01" || tag == "AI\x02"
}

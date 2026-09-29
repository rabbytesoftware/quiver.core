package archive

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/models"
)

type layout string

const (
	layoutTar    layout = "tar"
	layoutZip    layout = "zip"
	layoutSingle layout = "single"
)

const headSize = 512

type Archive struct {
	layout layout
	codec  codec
}

func Detect(
	src *os.File,
	size int64,
) (Archive, error) {
	if kind, ok := kindFromSuffix(src.Name()); ok {
		return kind, nil
	}

	return kindFromMagic(src, size)
}

func kindFromSuffix(
	path string,
) (Archive, bool) {
	lower := strings.ToLower(filepath.Base(path))
	for _, entry := range suffixTable() {
		if strings.HasSuffix(lower, entry.suffix) {
			return entry.kind, true
		}
	}

	return Archive{}, false
}

func suffixTable() []struct {
	suffix string
	kind   Archive
} {
	return []struct {
		suffix string
		kind   Archive
	}{
		{".tar.gz", Archive{layoutTar, codecGzip}},
		{".tgz", Archive{layoutTar, codecGzip}},
		{".tar.xz", Archive{layoutTar, codecXz}},
		{".txz", Archive{layoutTar, codecXz}},
		{".tar.bz2", Archive{layoutTar, codecBzip2}},
		{".tbz", Archive{layoutTar, codecBzip2}},
		{".tbz2", Archive{layoutTar, codecBzip2}},
		{".tar.zst", Archive{layoutTar, codecZstd}},
		{".tar", Archive{layoutTar, codecNone}},
		{".zip", Archive{layoutZip, codecNone}},
		{".gz", Archive{layoutSingle, codecGzip}},
		{".xz", Archive{layoutSingle, codecXz}},
		{".bz2", Archive{layoutSingle, codecBzip2}},
		{".zst", Archive{layoutSingle, codecZstd}},
	}
}

func kindFromMagic(
	src *os.File,
	size int64,
) (Archive, error) {
	head, err := readHead(io.NewSectionReader(src, 0, size))
	if err != nil {
		return Archive{}, fmt.Errorf("unpack: read %s: %w", src.Name(), err)
	}

	if bytes.HasPrefix(head, []byte("PK\x03\x04")) || bytes.HasPrefix(head, []byte("PK\x05\x06")) {
		return Archive{layoutZip, codecNone}, nil
	}

	if c := codecFromMagic(head); c != codecNone {
		return kindInsideCodec(src, size, c)
	}

	if isTarHeader(head) {
		return Archive{layoutTar, codecNone}, nil
	}

	return Archive{}, fmt.Errorf("unpack: %s: %w", src.Name(), models.ErrUnknownFormat)
}

func kindInsideCodec(
	src *os.File,
	size int64,
	c codec,
) (Archive, error) {
	rc, err := c.open(io.NewSectionReader(src, 0, size))
	if err != nil {
		return Archive{}, fmt.Errorf("unpack: %s: %w", src.Name(), err)
	}
	defer rc.Close() //nolint:errcheck

	head, err := readHead(rc)
	if err != nil {
		return Archive{}, fmt.Errorf("unpack: %s: %s: %w", src.Name(), c, err)
	}

	if isTarHeader(head) {
		return Archive{layoutTar, c}, nil
	}

	return Archive{layoutSingle, c}, nil
}

func readHead(
	r io.Reader,
) ([]byte, error) {
	head := make([]byte, headSize)

	n, err := io.ReadFull(r, head)
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return head[:n], nil
	}

	return head[:n], err
}

func isTarHeader(
	head []byte,
) bool {
	return len(head) >= 262 && string(head[257:262]) == "ustar"
}

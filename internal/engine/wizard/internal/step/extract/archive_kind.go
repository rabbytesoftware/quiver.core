package extract

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var ErrUnknownFormat = errors.New("extract: unknown archive format")

type layout string

const (
	layoutTar    layout = "tar"
	layoutZip    layout = "zip"
	layoutSingle layout = "single"
	layoutDmg    layout = "dmg"
)

const headSize = 512

type archiveKind struct {
	layout layout
	codec  codec
}

func detectKind(
	src *os.File,
	size int64,
) (archiveKind, error) {
	if kind, ok := kindFromSuffix(src.Name()); ok {
		return kind, nil
	}

	return kindFromMagic(src, size)
}

func kindFromSuffix(
	path string,
) (archiveKind, bool) {
	lower := strings.ToLower(filepath.Base(path))
	for _, entry := range suffixTable() {
		if strings.HasSuffix(lower, entry.suffix) {
			return entry.kind, true
		}
	}

	return archiveKind{}, false
}

func suffixTable() []struct {
	suffix string
	kind   archiveKind
} {
	return []struct {
		suffix string
		kind   archiveKind
	}{
		{".tar.gz", archiveKind{layoutTar, codecGzip}},
		{".tgz", archiveKind{layoutTar, codecGzip}},
		{".tar.xz", archiveKind{layoutTar, codecXz}},
		{".txz", archiveKind{layoutTar, codecXz}},
		{".tar.bz2", archiveKind{layoutTar, codecBzip2}},
		{".tbz", archiveKind{layoutTar, codecBzip2}},
		{".tbz2", archiveKind{layoutTar, codecBzip2}},
		{".tar.zst", archiveKind{layoutTar, codecZstd}},
		{".tar", archiveKind{layoutTar, codecNone}},
		{".zip", archiveKind{layoutZip, codecNone}},
		{".gz", archiveKind{layoutSingle, codecGzip}},
		{".xz", archiveKind{layoutSingle, codecXz}},
		{".bz2", archiveKind{layoutSingle, codecBzip2}},
		{".zst", archiveKind{layoutSingle, codecZstd}},
		{".dmg", archiveKind{layoutDmg, codecNone}},
	}
}

func kindFromMagic(
	src *os.File,
	size int64,
) (archiveKind, error) {
	head, err := readHead(io.NewSectionReader(src, 0, size))
	if err != nil {
		return archiveKind{}, fmt.Errorf("extract: read %s: %w", src.Name(), err)
	}

	if bytes.HasPrefix(head, []byte("PK\x03\x04")) || bytes.HasPrefix(head, []byte("PK\x05\x06")) {
		return archiveKind{layoutZip, codecNone}, nil
	}

	if c := codecFromMagic(head); c != codecNone {
		return kindInsideCodec(src, size, c)
	}

	if isTarHeader(head) {
		return archiveKind{layoutTar, codecNone}, nil
	}

	if isDmg(src, size) {
		return archiveKind{layoutDmg, codecNone}, nil
	}

	return archiveKind{}, fmt.Errorf("extract: %s: %w", src.Name(), ErrUnknownFormat)
}

func kindInsideCodec(
	src *os.File,
	size int64,
	c codec,
) (archiveKind, error) {
	rc, err := c.open(io.NewSectionReader(src, 0, size))
	if err != nil {
		return archiveKind{}, fmt.Errorf("extract: %s: %w", src.Name(), err)
	}
	defer rc.Close() //nolint:errcheck

	head, err := readHead(rc)
	if err != nil {
		return archiveKind{}, fmt.Errorf("extract: %s: %s: %w", src.Name(), c, err)
	}

	if isTarHeader(head) {
		return archiveKind{layoutTar, c}, nil
	}

	return archiveKind{layoutSingle, c}, nil
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

func isDmg(
	src io.ReaderAt,
	size int64,
) bool {
	trailer := make([]byte, 4)
	_, err := src.ReadAt(trailer, size-headSize)

	return err == nil && string(trailer) == "koly"
}

func (k archiveKind) extract(
	ctx context.Context,
	src *os.File,
	size int64,
	g *guard,
) error {
	switch k.layout {
	case layoutZip:
		return extractZip(ctx, src, size, g)
	case layoutDmg:
		return extractDmg(ctx, src.Name(), g)
	case layoutTar, layoutSingle:
	}

	rc, err := k.codec.open(io.NewSectionReader(src, 0, size))
	if err != nil {
		return fmt.Errorf("extract: %s: %w", src.Name(), err)
	}
	defer rc.Close() //nolint:errcheck

	if k.layout == layoutTar {
		return extractTar(ctx, rc, g)
	}

	return extractSingle(ctx, rc, singleName(src.Name()), g)
}

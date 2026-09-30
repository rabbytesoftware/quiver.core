package archive

import (
	"bytes"
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

type layout string

const (
	layoutTar    layout = "tar"
	layoutZip    layout = "zip"
	layoutSingle layout = "single"
)

const (
	headSize     = 512
	bundleSuffix = ".app"
)

type archive struct {
	src      *os.File
	size     int64
	layout   layout
	codec    codec
	maxBytes int64
	rules    guard.HostRules
}

func New(
	maxBytes int64,
	rules guard.HostRules,
) models.Detect {
	return func(src *os.File, size int64) (models.Format, bool, error) {
		a, err := detect(src, size)
		if errors.Is(err, models.ErrUnknownFormat) {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, err
		}

		a.maxBytes, a.rules = maxBytes, rules
		return a, true, nil
	}
}

func (a *archive) Kind() models.Kind {
	return models.KindArchive
}

func (a *archive) Unit() models.Unit {
	return models.Unit{}
}

func (a *archive) Unpack(
	ctx context.Context,
	target models.Target,
) (models.Result, error) {
	g, err := guard.Open(ctx, target.Dir, a.maxBytes, guard.WithHostRules(a.rules))
	if err != nil {
		return models.Result{}, err
	}
	defer g.Close()

	if err := errors.Join(a.extract(ctx, g, target.Name), g.Verify()); err != nil {
		return models.Result{}, err
	}

	return models.Result{Apps: g.Apps(target.Dir, bundleSuffix, true)}, nil
}

func detect(
	src *os.File,
	size int64,
) (*archive, error) {
	if a, ok := fromSuffix(src.Name()); ok {
		return a.bind(src, size), nil
	}

	a, err := fromMagic(src, size)
	if err != nil {
		return nil, err
	}

	return a.bind(src, size), nil
}

func (a archive) bind(
	src *os.File,
	size int64,
) *archive {
	a.src, a.size = src, size
	return &a
}

func fromSuffix(
	path string,
) (archive, bool) {
	lower := strings.ToLower(filepath.Base(path))
	for _, entry := range suffixTable() {
		if strings.HasSuffix(lower, entry.suffix) {
			return entry.archive, true
		}
	}

	return archive{}, false
}

func suffixTable() []struct {
	suffix  string
	archive archive
} {
	return []struct {
		suffix  string
		archive archive
	}{
		{".tar.gz", archive{layout: layoutTar, codec: codecGzip}},
		{".tgz", archive{layout: layoutTar, codec: codecGzip}},
		{".tar.xz", archive{layout: layoutTar, codec: codecXz}},
		{".txz", archive{layout: layoutTar, codec: codecXz}},
		{".tar.bz2", archive{layout: layoutTar, codec: codecBzip2}},
		{".tbz", archive{layout: layoutTar, codec: codecBzip2}},
		{".tbz2", archive{layout: layoutTar, codec: codecBzip2}},
		{".tar.zst", archive{layout: layoutTar, codec: codecZstd}},
		{".tar", archive{layout: layoutTar, codec: codecNone}},
		{".zip", archive{layout: layoutZip, codec: codecNone}},
		{".gz", archive{layout: layoutSingle, codec: codecGzip}},
		{".xz", archive{layout: layoutSingle, codec: codecXz}},
		{".bz2", archive{layout: layoutSingle, codec: codecBzip2}},
		{".zst", archive{layout: layoutSingle, codec: codecZstd}},
	}
}

func fromMagic(
	src *os.File,
	size int64,
) (archive, error) {
	head, err := readHead(io.NewSectionReader(src, 0, size))
	if err != nil {
		return archive{}, fmt.Errorf("unpack: read %s: %w", src.Name(), err)
	}

	if bytes.HasPrefix(head, []byte("PK\x03\x04")) || bytes.HasPrefix(head, []byte("PK\x05\x06")) {
		return archive{layout: layoutZip, codec: codecNone}, nil
	}

	if c := codecFromMagic(head); c != codecNone {
		return insideCodec(src, size, c)
	}

	if isTarHeader(head) {
		return archive{layout: layoutTar, codec: codecNone}, nil
	}

	return archive{}, fmt.Errorf("unpack: %s: %w", src.Name(), models.ErrUnknownFormat)
}

func insideCodec(
	src *os.File,
	size int64,
	c codec,
) (archive, error) {
	rc, err := c.open(io.NewSectionReader(src, 0, size))
	if err != nil {
		return archive{}, fmt.Errorf("unpack: %s: %w", src.Name(), err)
	}
	defer rc.Close() //nolint:errcheck

	head, err := readHead(rc)
	if err != nil {
		return archive{}, fmt.Errorf("unpack: %s: %s: %w", src.Name(), c, err)
	}

	if isTarHeader(head) {
		return archive{layout: layoutTar, codec: c}, nil
	}

	return archive{layout: layoutSingle, codec: c}, nil
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

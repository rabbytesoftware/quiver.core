package unpack

import (
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
)

const zstdMaxWindow = 128 << 20

type codec string

const (
	codecNone  codec = "none"
	codecGzip  codec = "gzip"
	codecXz    codec = "xz"
	codecBzip2 codec = "bzip2"
	codecZstd  codec = "zstd"
)

func codecFromMagic(
	head []byte,
) codec {
	switch {
	case bytes.HasPrefix(head, []byte{0x1f, 0x8b}):
		return codecGzip
	case bytes.HasPrefix(head, []byte{0xfd, '7', 'z', 'X', 'Z', 0x00}):
		return codecXz
	case bytes.HasPrefix(head, []byte("BZh")):
		return codecBzip2
	case bytes.HasPrefix(head, []byte{0x28, 0xb5, 0x2f, 0xfd}):
		return codecZstd
	}

	return codecNone
}

func (c codec) open(
	r io.Reader,
) (io.ReadCloser, error) {
	switch c {
	case codecGzip:
		return openGzip(r)
	case codecXz:
		return openXz(r)
	case codecBzip2:
		return io.NopCloser(bzip2.NewReader(r)), nil
	case codecZstd:
		return openZstd(r)
	case codecNone:
	}

	return io.NopCloser(r), nil
}

func openGzip(
	r io.Reader,
) (io.ReadCloser, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("gzip: %w", err)
	}

	return gz, nil
}

func openXz(
	r io.Reader,
) (io.ReadCloser, error) {
	xr, err := xz.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("xz: %w", err)
	}

	return io.NopCloser(xr), nil
}

func openZstd(
	r io.Reader,
) (io.ReadCloser, error) {
	dec, err := zstd.NewReader(r, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxWindow(zstdMaxWindow))
	if err != nil {
		return nil, fmt.Errorf("zstd: %w", err)
	}

	return dec.IOReadCloser(), nil
}

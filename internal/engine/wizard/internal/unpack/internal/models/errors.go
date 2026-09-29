package models

import "errors"

var (
	ErrEscape              = errors.New("unpack: entry escapes destination")
	ErrTooLarge            = errors.New("unpack: uncompressed size exceeds limit")
	ErrTooMany             = errors.New("unpack: entry count exceeds limit")
	ErrUnknownFormat       = errors.New("unpack: unknown archive format")
	ErrUnsupportedAppImage = errors.New("unpack: unsupported appimage type")
	ErrNoSquashfs          = errors.New("unpack: appimage has no squashfs image")
)

package models

import "errors"

var (
	ErrEscape              = errors.New("unpack: entry escapes destination")
	ErrTooLarge            = errors.New("unpack: uncompressed size exceeds limit")
	ErrTooMany             = errors.New("unpack: entry count exceeds limit")
	ErrUnknownFormat       = errors.New("unpack: unknown format")
	ErrUnsupportedAppImage = errors.New("unpack: unsupported appimage type")
	ErrNoSquashfs          = errors.New("unpack: appimage has no squashfs image")
	ErrReservedName        = errors.New("unpack: entry name is not allowed on this platform")
	ErrNameCollision       = errors.New("unpack: entry names collide on a case-insensitive filesystem")
	ErrUnsupportedPlatform = errors.New("unpack: format is not supported on this platform")
	ErrMsiexecFailed       = errors.New("unpack: msiexec failed")
)

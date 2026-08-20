package updater

import "errors"

var (
	ErrInvalidLayout = errors.New("updater: invalid layout")
	ErrInvalidState  = errors.New("updater: invalid state")
	ErrUnsafePath    = errors.New("updater: unsafe path")
)

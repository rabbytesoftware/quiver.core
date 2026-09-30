package expose

import "errors"

var (
	ErrRefused     = errors.New("expose: entry refused")
	ErrUnknownKind = errors.New("expose: unknown entry kind")
)

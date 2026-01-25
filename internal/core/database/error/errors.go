package error

import "errors"

var (
	ErrNameRequired = errors.New("NAME_IS_REQUIRED")
)

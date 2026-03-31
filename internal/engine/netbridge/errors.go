package netbridge

import "errors"

var (
	ErrNoPortAvailable = errors.New("netbridge: no available port found")
	ErrPortOutOfRange  = errors.New("netbridge: port out of valid range")
	ErrBuildIncomplete = errors.New("netbridge: builder missing required dependencies")
)

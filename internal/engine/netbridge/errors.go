package netbridge

import "errors"

var (
	ErrNoPortAvailable = errors.New("netbridge: no available port found")
	ErrPortOutOfRange  = errors.New("netbridge: port out of valid range")
	ErrBuildFailed     = errors.New("netbridge: failed to build netbridge")
)

package models

import "errors"

var (
	ErrEmptyCommand   = errors.New("command cannot be empty")
	ErrInvalidState   = errors.New("invalid process state for operation")
	ErrNoProcess      = errors.New("no underlying OS process")
	ErrKillTimeout    = errors.New("timeout waiting for process to exit after kill")
	ErrInvalidTimeout = errors.New("timeout must be non-negative")
	ErrInvalidSignal  = errors.New("unsupported signal kind")
)

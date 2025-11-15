package models

import "errors"

var (
	ErrUnsupportedOS 		= errors.New("unsupported operating system")
	ErrProcessNotFound 	= errors.New("process not found")
	ErrEmptyCommand 		= errors.New("command cannot be empty")
	ErrInvalidState 		= errors.New("invalid process state for operation")
	ErrNoProcess 		= errors.New("no underlying OS process")
	ErrKillTimeout 		= errors.New("timeout waiting for process to exit after kill")
	ErrAlreadyStarted 	= errors.New("process already started")
	ErrNotRunning 		= errors.New("process is not running")
	ErrAlreadyFinished 	= errors.New("process has already finished")
)


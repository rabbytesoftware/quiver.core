package models

import "time"

type Status string

const (
	StatusPrepared Status = "prepared"
	StatusRunning Status = "running"
	StatusStopping Status = "stopping"
	StatusKilling Status = "killing"
	StatusFinished Status = "finished"
)

func (s Status) String() string {
	return string(s)
}

func (s Status) IsValid() bool {
	switch s {
	case StatusPrepared, StatusRunning, StatusStopping, StatusKilling, StatusFinished:
		return true
	default:
		return false
	}
}

func (s Status) IsTerminal() bool {
	return s == StatusFinished
}

func (s Status) IsActive() bool {
	return s == StatusRunning || s == StatusStopping || s == StatusKilling
}

type Config struct {
	Command        []string
	WorkDir        string
	Env            map[string]string
	Timeout        time.Duration      // Timeout for process execution (0 = no timeout)
	KillTimeout    time.Duration      // Timeout to wait after kill signal (default: 30s)
	StopTimeout    time.Duration      // Timeout to wait after stop signal (default: 30s)
	BufferSize     int
}

func NewConfig(command []string) *Config {
	return &Config{
		Command:     command,
		WorkDir:     ".",
		Env:         make(map[string]string),
		Timeout:     0,
		KillTimeout: 30 * time.Second,
		StopTimeout: 30 * time.Second,
		BufferSize:  0,
	}
}

func (c *Config) Validate() error {
	if len(c.Command) == 0 {
		return ErrEmptyCommand
	}
	return nil
}

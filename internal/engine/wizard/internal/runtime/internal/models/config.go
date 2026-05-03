package models

import "time"

type Config struct {
	Command     []string
	WorkDir     string
	Env         map[string]string
	Timeout     time.Duration
	KillTimeout time.Duration
	StopTimeout time.Duration
	BufferSize  int
	ShellWrap   bool
}

func NewConfig(
	command []string,
) *Config {
	return &Config{
		Command:     command,
		WorkDir:     ".",
		Env:         make(map[string]string),
		KillTimeout: 30 * time.Second,
		StopTimeout: 30 * time.Second,
	}
}

func (c *Config) Validate() error {
	if len(c.Command) == 0 {
		return ErrEmptyCommand
	}
	if c.Timeout < 0 || c.KillTimeout < 0 || c.StopTimeout < 0 {
		return ErrInvalidTimeout
	}
	return nil
}

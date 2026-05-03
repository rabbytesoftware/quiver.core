package models

import (
	"testing"
	"time"
)

func TestNewConfig_Defaults(t *testing.T) {
	cmd := []string{"echo", "hello"}
	c := NewConfig(cmd)

	if len(c.Command) != 2 || c.Command[0] != "echo" {
		t.Errorf("Command = %v, want %v", c.Command, cmd)
	}
	if c.WorkDir != "." {
		t.Errorf("WorkDir = %q, want \".\"", c.WorkDir)
	}
	if c.Env == nil {
		t.Error("Env should be initialized")
	}
	if c.KillTimeout != 30*time.Second {
		t.Errorf("KillTimeout = %v, want 30s", c.KillTimeout)
	}
	if c.StopTimeout != 30*time.Second {
		t.Errorf("StopTimeout = %v, want 30s", c.StopTimeout)
	}
}

func TestConfig_Validate_EmptyCommand(t *testing.T) {
	c := &Config{Command: []string{}}
	if err := c.Validate(); err != ErrEmptyCommand {
		t.Errorf("Validate() = %v, want ErrEmptyCommand", err)
	}
}

func TestConfig_Validate_NegativeTimeout(t *testing.T) {
	c := NewConfig([]string{"echo"})
	c.Timeout = -1

	if err := c.Validate(); err != ErrInvalidTimeout {
		t.Errorf("Validate() = %v, want ErrInvalidTimeout", err)
	}
}

func TestConfig_Validate_NegativeKillTimeout(t *testing.T) {
	c := NewConfig([]string{"echo"})
	c.KillTimeout = -1

	if err := c.Validate(); err != ErrInvalidTimeout {
		t.Errorf("Validate() = %v, want ErrInvalidTimeout", err)
	}
}

func TestConfig_Validate_NegativeStopTimeout(t *testing.T) {
	c := NewConfig([]string{"echo"})
	c.StopTimeout = -1

	if err := c.Validate(); err != ErrInvalidTimeout {
		t.Errorf("Validate() = %v, want ErrInvalidTimeout", err)
	}
}

func TestConfig_Validate_Valid(t *testing.T) {
	c := NewConfig([]string{"echo"})
	if err := c.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

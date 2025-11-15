package models

import (
	"testing"
	"time"
)

func TestStatus_String(t *testing.T) {
	tests := []struct {
		name   string
		status Status
		want   string
	}{
		{"prepared", StatusPrepared, "prepared"},
		{"running", StatusRunning, "running"},
		{"stopping", StatusStopping, "stopping"},
		{"killing", StatusKilling, "killing"},
		{"finished", StatusFinished, "finished"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.String(); got != tt.want {
				t.Errorf("Status.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStatus_IsValid(t *testing.T) {
	tests := []struct {
		name   string
		status Status
		want   bool
	}{
		{"prepared is valid", StatusPrepared, true},
		{"running is valid", StatusRunning, true},
		{"stopping is valid", StatusStopping, true},
		{"killing is valid", StatusKilling, true},
		{"finished is valid", StatusFinished, true},
		{"empty is invalid", Status(""), false},
		{"random is invalid", Status("random"), false},
		{"invalid is invalid", Status("invalid"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.IsValid(); got != tt.want {
				t.Errorf("Status.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStatus_IsTerminal(t *testing.T) {
	tests := []struct {
		name   string
		status Status
		want   bool
	}{
		{"prepared is not terminal", StatusPrepared, false},
		{"running is not terminal", StatusRunning, false},
		{"stopping is not terminal", StatusStopping, false},
		{"killing is not terminal", StatusKilling, false},
		{"finished is terminal", StatusFinished, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.IsTerminal(); got != tt.want {
				t.Errorf("Status.IsTerminal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStatus_IsActive(t *testing.T) {
	tests := []struct {
		name   string
		status Status
		want   bool
	}{
		{"prepared is not active", StatusPrepared, false},
		{"running is active", StatusRunning, true},
		{"stopping is active", StatusStopping, true},
		{"killing is active", StatusKilling, true},
		{"finished is not active", StatusFinished, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.IsActive(); got != tt.want {
				t.Errorf("Status.IsActive() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewConfig(t *testing.T) {
	command := []string{"echo", "test"}
	config := NewConfig(command)

	if config == nil {
		t.Fatal("NewConfig() returned nil")
	}

	if len(config.Command) != len(command) {
		t.Errorf("Command length = %d, want %d", len(config.Command), len(command))
	}

	for i, cmd := range command {
		if config.Command[i] != cmd {
			t.Errorf("Command[%d] = %s, want %s", i, config.Command[i], cmd)
		}
	}

	if config.WorkDir != "." {
		t.Errorf("WorkDir = %s, want '.'", config.WorkDir)
	}

	if config.Env == nil {
		t.Error("Env should not be nil")
	}

	if len(config.Env) != 0 {
		t.Errorf("Env length = %d, want 0", len(config.Env))
	}

	if config.Timeout != 0 {
		t.Errorf("Timeout = %v, want 0", config.Timeout)
	}

	if config.BufferSize != 0 {
		t.Errorf("BufferSize = %d, want 0", config.BufferSize)
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errType error
	}{
		{
			name: "valid config",
			config: &Config{
				Command: []string{"echo", "test"},
				WorkDir: "/tmp",
				Env:     map[string]string{"KEY": "value"},
			},
			wantErr: false,
		},
		{
			name: "empty command",
			config: &Config{
				Command: []string{},
				WorkDir: "/tmp",
			},
			wantErr: true,
			errType: ErrEmptyCommand,
		},
		{
			name: "nil command",
			config: &Config{
				Command: nil,
				WorkDir: "/tmp",
			},
			wantErr: true,
			errType: ErrEmptyCommand,
		},
		{
			name: "minimal valid config",
			config: &Config{
				Command: []string{"ls"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != tt.errType {
				t.Errorf("Config.Validate() error = %v, want %v", err, tt.errType)
			}
		})
	}
}

func TestConfig_Mutations(t *testing.T) {
	config := NewConfig([]string{"echo", "test"})

	// Test WorkDir modification
	config.WorkDir = "/custom/path"
	if config.WorkDir != "/custom/path" {
		t.Errorf("WorkDir = %s, want '/custom/path'", config.WorkDir)
	}

	// Test Env modification
	config.Env["TEST_VAR"] = "test_value"
	if config.Env["TEST_VAR"] != "test_value" {
		t.Errorf("Env[TEST_VAR] = %s, want 'test_value'", config.Env["TEST_VAR"])
	}

	// Test Timeout modification
	config.Timeout = 30 * time.Second
	if config.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", config.Timeout)
	}

	// Test BufferSize modification
	config.BufferSize = 4096
	if config.BufferSize != 4096 {
		t.Errorf("BufferSize = %d, want 4096", config.BufferSize)
	}
}

func TestConfig_EnvIsolation(t *testing.T) {
	config1 := NewConfig([]string{"echo", "test1"})
	config2 := NewConfig([]string{"echo", "test2"})

	config1.Env["VAR1"] = "value1"
	config2.Env["VAR2"] = "value2"

	if _, exists := config2.Env["VAR1"]; exists {
		t.Error("config2 should not have VAR1 from config1")
	}

	if _, exists := config1.Env["VAR2"]; exists {
		t.Error("config1 should not have VAR2 from config2")
	}
}

package models

import (
	"errors"
	"testing"
)

func TestErrors_Defined(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "ErrUnsupportedOS",
			err:  ErrUnsupportedOS,
			want: "unsupported operating system",
		},
		{
			name: "ErrProcessNotFound",
			err:  ErrProcessNotFound,
			want: "process not found",
		},
		{
			name: "ErrEmptyCommand",
			err:  ErrEmptyCommand,
			want: "command cannot be empty",
		},
		{
			name: "ErrInvalidState",
			err:  ErrInvalidState,
			want: "invalid process state for operation",
		},
		{
			name: "ErrNoProcess",
			err:  ErrNoProcess,
			want: "no underlying OS process",
		},
		{
			name: "ErrKillTimeout",
			err:  ErrKillTimeout,
			want: "timeout waiting for process to exit after kill",
		},
		{
			name: "ErrAlreadyStarted",
			err:  ErrAlreadyStarted,
			want: "process already started",
		},
		{
			name: "ErrNotRunning",
			err:  ErrNotRunning,
			want: "process is not running",
		},
		{
			name: "ErrAlreadyFinished",
			err:  ErrAlreadyFinished,
			want: "process has already finished",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				t.Fatalf("expected error to be defined, got nil")
			}

			if got := tt.err.Error(); got != tt.want {
				t.Errorf("error message = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestErrors_IsComparable(t *testing.T) {
	tests := []struct {
		name  string
		err1  error
		err2  error
		equal bool
	}{
		{
			name:  "same error",
			err1:  ErrUnsupportedOS,
			err2:  ErrUnsupportedOS,
			equal: true,
		},
		{
			name:  "different errors",
			err1:  ErrUnsupportedOS,
			err2:  ErrProcessNotFound,
			equal: false,
		},
		{
			name:  "wrapped error with errors.Is",
			err1:  errors.New("wrapped: " + ErrEmptyCommand.Error()),
			err2:  ErrEmptyCommand,
			equal: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if (tt.err1 == tt.err2) != tt.equal {
				t.Errorf("error comparison = %v, want %v", tt.err1 == tt.err2, tt.equal)
			}
		})
	}
}

func TestErrors_UniqueErrors(t *testing.T) {
	errorMap := make(map[error]bool)
	allErrors := []error{
		ErrUnsupportedOS,
		ErrProcessNotFound,
		ErrEmptyCommand,
		ErrInvalidState,
		ErrNoProcess,
		ErrKillTimeout,
		ErrAlreadyStarted,
		ErrNotRunning,
		ErrAlreadyFinished,
	}

	for _, err := range allErrors {
		if errorMap[err] {
			t.Errorf("duplicate error found: %v", err)
		}
		errorMap[err] = true
	}

	if len(errorMap) != len(allErrors) {
		t.Errorf("expected %d unique errors, got %d", len(allErrors), len(errorMap))
	}
}

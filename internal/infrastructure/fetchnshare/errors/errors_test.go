package errors

import (
	"errors"
	"fmt"
	"testing"
)

func TestFNSError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *FNSError
		expected string
	}{
		{
			name: "with path",
			err: &FNSError{
				Op:   "Read",
				Path: "/path/to/file",
				Err:  fmt.Errorf("permission denied"),
			},
			expected: "Read /path/to/file: permission denied",
		},
		{
			name: "without path",
			err: &FNSError{
				Op:  "Fetch",
				Err: fmt.Errorf("network timeout"),
			},
			expected: "Fetch: network timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.expected {
				t.Errorf("Error() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestFNSError_Unwrap(t *testing.T) {
	underlying := fmt.Errorf("underlying error")
	err := &FNSError{
		Op:   "Write",
		Path: "/tmp/test",
		Err:  underlying,
	}

	unwrapped := err.Unwrap()
	if unwrapped != underlying {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, underlying)
	}

	if !errors.Is(err, underlying) {
		t.Error("errors.Is() should work with FNSError")
	}
}

func TestOp(t *testing.T) {
	underlying := fmt.Errorf("disk full")
	err := Op("Write", "/tmp/file", underlying)

	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	fnsErr, ok := err.(*FNSError)
	if !ok {
		t.Fatal("Expected *FNSError")
	}

	if fnsErr.Op != "Write" {
		t.Errorf("Op = %q, want %q", fnsErr.Op, "Write")
	}

	if fnsErr.Path != "/tmp/file" {
		t.Errorf("Path = %q, want %q", fnsErr.Path, "/tmp/file")
	}

	if !errors.Is(err, underlying) {
		t.Error("Should preserve underlying error")
	}
}

func TestOp_Nil(t *testing.T) {
	err := Op("Read", "/path", nil)
	if err != nil {
		t.Errorf("Expected nil for nil error, got %v", err)
	}
}

func TestNotFound(t *testing.T) {
	err := NotFound("GetInfo", "/missing/file")

	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	fnsErr, ok := err.(*FNSError)
	if !ok {
		t.Fatal("Expected *FNSError")
	}

	if fnsErr.Op != "GetInfo" {
		t.Errorf("Op = %q, want %q", fnsErr.Op, "GetInfo")
	}

	if fnsErr.Path != "/missing/file" {
		t.Errorf("Path = %q, want %q", fnsErr.Path, "/missing/file")
	}

	expected := "GetInfo /missing/file: not found"
	if fnsErr.Error() != expected {
		t.Errorf("Error() = %q, want %q", fnsErr.Error(), expected)
	}
}

func TestUnsupported(t *testing.T) {
	err := Unsupported("Mkdir", "remote URLs")

	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	fnsErr, ok := err.(*FNSError)
	if !ok {
		t.Fatal("Expected *FNSError")
	}

	if fnsErr.Op != "Mkdir" {
		t.Errorf("Op = %q, want %q", fnsErr.Op, "Mkdir")
	}

	expected := "Mkdir: not supported for remote URLs"
	if fnsErr.Error() != expected {
		t.Errorf("Error() = %q, want %q", fnsErr.Error(), expected)
	}
}

func TestErrorChaining(t *testing.T) {
	baseErr := fmt.Errorf("base error")
	wrappedErr := fmt.Errorf("wrapped: %w", baseErr)
	fnsErr := Op("Copy", "/src", wrappedErr)

	if !errors.Is(fnsErr, baseErr) {
		t.Error("errors.Is() should work through multiple wrapping levels")
	}
}

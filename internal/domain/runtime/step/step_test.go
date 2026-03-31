package step

import (
	"testing"
	"time"
)

func TestNewRunStep(t *testing.T) {
	s := NewRunStep("run title", "echo hello", 5*time.Second, true)
	if s.Type() != StepTypeRun {
		t.Errorf("Type() = %v, want %v", s.Type(), StepTypeRun)
	}
	if s.Title != "run title" {
		t.Errorf("Title = %v, want run title", s.Title)
	}
	if !s.ExitOnFailure {
		t.Error("ExitOnFailure = false, want true")
	}
	if s.Command.Default != "echo hello" {
		t.Errorf("Command.Default = %v, want echo hello", s.Command.Default)
	}
	if s.Timeout.Default != "5s" {
		t.Errorf("Timeout.Default = %v, want 5s", s.Timeout.Default)
	}
}

func TestNewFetchStep(t *testing.T) {
	s := NewFetchStep("fetch title", "https://example.com/file", "/tmp/file", 10*time.Second, false)
	if s.Type() != StepTypeFetch {
		t.Errorf("Type() = %v, want %v", s.Type(), StepTypeFetch)
	}
	if s.Title != "fetch title" {
		t.Errorf("Title = %v, want fetch title", s.Title)
	}
	if s.ExitOnFailure {
		t.Error("ExitOnFailure = true, want false")
	}
	if s.URL.Default != "https://example.com/file" {
		t.Errorf("URL.Default = %v, want https://example.com/file", s.URL.Default)
	}
	if s.To.Default != "/tmp/file" {
		t.Errorf("To.Default = %v, want /tmp/file", s.To.Default)
	}
}

func TestNewSignalStep(t *testing.T) {
	s := NewSignalStep("signal title", "SIGTERM", 3*time.Second, false)
	if s.Type() != StepTypeSignal {
		t.Errorf("Type() = %v, want %v", s.Type(), StepTypeSignal)
	}
	if s.Title != "signal title" {
		t.Errorf("Title = %v, want signal title", s.Title)
	}
	if s.Signal.Default != "SIGTERM" {
		t.Errorf("Signal.Default = %v, want SIGTERM", s.Signal.Default)
	}
}

func TestNewDependenciesStep(t *testing.T) {
	s := NewDependenciesStep("deps title")
	if s.Type() != StepTypeDependencies {
		t.Errorf("Type() = %v, want %v", s.Type(), StepTypeDependencies)
	}
	if s.Title != "deps title" {
		t.Errorf("Title = %v, want deps title", s.Title)
	}
}

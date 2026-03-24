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
	if s.Title() != "run title" {
		t.Errorf("Title() = %v, want run title", s.Title())
	}
	if !s.ExitOnFailure() {
		t.Error("ExitOnFailure() = false, want true")
	}
	if s.Command != "echo hello" {
		t.Errorf("Command = %v, want echo hello", s.Command)
	}
	if s.Timeout != 5*time.Second {
		t.Errorf("Timeout = %v, want 5s", s.Timeout)
	}
}

func TestNewFetchStep(t *testing.T) {
	s := NewFetchStep("fetch title", "https://example.com/file", "/tmp/file", 10*time.Second, false)
	if s.Type() != StepTypeFetch {
		t.Errorf("Type() = %v, want %v", s.Type(), StepTypeFetch)
	}
	if s.Title() != "fetch title" {
		t.Errorf("Title() = %v, want fetch title", s.Title())
	}
	if s.ExitOnFailure() {
		t.Error("ExitOnFailure() = true, want false")
	}
	if s.URL != "https://example.com/file" {
		t.Errorf("URL = %v, want https://example.com/file", s.URL)
	}
	if s.To != "/tmp/file" {
		t.Errorf("To = %v, want /tmp/file", s.To)
	}
}

func TestNewSignalStep(t *testing.T) {
	s := NewSignalStep("signal title", "SIGTERM", 3*time.Second, false)
	if s.Type() != StepTypeSignal {
		t.Errorf("Type() = %v, want %v", s.Type(), StepTypeSignal)
	}
	if s.Title() != "signal title" {
		t.Errorf("Title() = %v, want signal title", s.Title())
	}
	if s.Signal != "SIGTERM" {
		t.Errorf("Signal = %v, want SIGTERM", s.Signal)
	}
}

func TestNewDependenciesStep(t *testing.T) {
	s := NewDependenciesStep("deps title")
	if s.Type() != StepTypeDependencies {
		t.Errorf("Type() = %v, want %v", s.Type(), StepTypeDependencies)
	}
	if s.Title() != "deps title" {
		t.Errorf("Title() = %v, want deps title", s.Title())
	}
	if !s.ExitOnFailure() {
		t.Error("ExitOnFailure() = false, want true (dependencies always exit on failure)")
	}
}

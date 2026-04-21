package models

import (
	"strings"
	"testing"
)

func TestAmbiguousTargetError_Error(t *testing.T) {
	e := &AmbiguousTargetError{
		Key1: "linux/amd64",
		Key2: "linux/*",
		OS:   "linux/amd64",
	}

	msg := e.Error()

	if msg == "" {
		t.Fatal("expected non-empty error message")
	}
	if !strings.Contains(msg, "linux/amd64") {
		t.Errorf("expected error to contain OS %q, got: %s", "linux/amd64", msg)
	}
	if !strings.Contains(msg, "linux/*") {
		t.Errorf("expected error to contain key2 %q, got: %s", "linux/*", msg)
	}
}

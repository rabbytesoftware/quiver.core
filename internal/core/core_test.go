package core

import (
	"testing"
)

func TestNew(t *testing.T) {
	core := New()

	if core == nil {
		t.Fatal("New() returned nil")
	}

	if core.metadata == nil {
		t.Error("metadata is not initialized")
	}

	if core.config == nil {
		t.Error("config is not initialized")
	}
}

func TestCore_GetMetadata(t *testing.T) {
	core := New()
	metadata := core.GetMetadata()

	if metadata == nil {
		t.Error("GetMetadata() returned nil")
	}

	metadata2 := core.GetMetadata()
	if metadata != metadata2 {
		t.Error("GetMetadata() should return the same instance")
	}
}

func TestCore_GetConfig(t *testing.T) {
	core := New()
	config := core.GetConfig()

	if config == nil {
		t.Error("GetConfig() returned nil")
	}

	config2 := core.GetConfig()
	if config != config2 {
		t.Error("GetConfig() should return the same instance")
	}
}

func TestCoreStructure(t *testing.T) {
	core := New()

	if core.metadata == nil {
		t.Error("Core.metadata field is nil")
	}

	if core.config == nil {
		t.Error("Core.config field is nil")
	}
}

func TestCoreInitialization(t *testing.T) {
	core1 := New()
	core2 := New()

	if core1 == core2 {
		t.Error("New() should create new instances each time")
	}

	if core1.GetMetadata() != core2.GetMetadata() {
		t.Error("Metadata should be singleton across Core instances")
	}

	if core1.GetConfig() != core2.GetConfig() {
		t.Error("Config should be singleton across Core instances")
	}
}

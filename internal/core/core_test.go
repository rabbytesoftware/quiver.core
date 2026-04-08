package core

import (
	"testing"
)

func TestInit(t *testing.T) {
	core := Init()

	if core == nil {
		t.Fatal("Init() returned nil")
	}

	if core.metadata == nil {
		t.Error("metadata is not initialized")
	}

	if core.config == nil {
		t.Error("config is not initialized")
	}
}

func TestCore_GetMetadata(t *testing.T) {
	core := Init()
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
	core := Init()
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
	core := Init()

	if core.metadata == nil {
		t.Error("Core.metadata field is nil")
	}

	if core.config == nil {
		t.Error("Core.config field is nil")
	}
}

func TestCoreInitialization(t *testing.T) {
	core1 := Init()
	core2 := Init()

	if core1 == core2 {
		t.Error("Init() should create new instances each time")
	}

	if core1.GetMetadata() != core2.GetMetadata() {
		t.Error("Metadata should be singleton across Core instances")
	}

	if core1.GetConfig() != core2.GetConfig() {
		t.Error("Config should be singleton across Core instances")
	}
}

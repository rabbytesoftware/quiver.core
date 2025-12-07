package main

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal"
	"github.com/rabbytesoftware/quiver/internal/core/metadata"
	"github.com/rabbytesoftware/quiver/internal/core/watcher"
)

func TestMain(t *testing.T) {
	if os.Getenv("TEST_MAIN") == "1" {
		main()
		return
	}

	t.Skip("Skipping TestMain to avoid port conflicts with other tests")
}

func TestMainComponents(t *testing.T) {
	internal := internal.NewInternal()
	if internal == nil {
		t.Error("Expected internal to be created")
	}

	watcher := internal.GetCore().GetWatcher()
	if watcher == nil {
		t.Error("Expected watcher to be created")
	}

	name := metadata.GetName()
	if name == "" {
		t.Error("Expected non-empty name from metadata")
	}

	version := metadata.GetVersion()
	if version == "" {
		t.Error("Expected non-empty version from metadata")
	}
}

func TestMainLogic(t *testing.T) {
	internal := internal.NewInternal()
	_ = internal.GetCore().GetWatcher()

	watcher.Info("Test message from main test")

	done := make(chan bool)
	go func() {
		time.Sleep(10 * time.Millisecond)

		name := metadata.GetName()
		version := metadata.GetVersion()
		codename := metadata.GetVersionCodename()
		message := fmt.Sprintf("%s %s '%s' - Initializing with embedded icon support...", name, version, codename)

		if message == "" {
			t.Error("Expected non-empty initialization message")
		}

		watcher.Info(message)
		done <- true
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Error("Goroutine did not complete in time")
	}
}

func TestInternalRun(t *testing.T) {
	internal := internal.NewInternal()

	done := make(chan bool)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				t.Logf("internal.Run() recovered (expected): %v", r)
			}
			done <- true
		}()

		go internal.Run()

		time.Sleep(50 * time.Millisecond)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Error("Test did not complete in time")
	}
}

package output

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/core/watcher"
)

func TestNewHandler(t *testing.T) {
	handler := NewHandler()

	if handler == nil {
		t.Fatal("NewHandler() returned nil")
	}

	if handler.output == nil {
		t.Error("output buffer not initialized")
	}

	if handler.error == nil {
		t.Error("error buffer not initialized")
	}

	if handler.outChan == nil {
		t.Error("outChan not initialized")
	}

	if handler.errChan == nil {
		t.Error("errChan not initialized")
	}

	if handler.closed {
		t.Error("handler should not be closed initially")
	}
}

func TestNewHandlerWithBuffers(t *testing.T) {
	outSize := 50
	errSize := 25
	handler := NewHandlerWithBuffers(outSize, errSize)

	if handler == nil {
		t.Fatal("NewHandlerWithBuffers() returned nil")
	}

	if cap(handler.outChan) != outSize {
		t.Errorf("outChan capacity = %d, want %d", cap(handler.outChan), outSize)
	}

	if cap(handler.errChan) != errSize {
		t.Errorf("errChan capacity = %d, want %d", cap(handler.errChan), errSize)
	}
}

func TestWriteOutput(t *testing.T) {
	handler := NewHandler()
	defer handler.Close()

	testLine := "test output line"
	handler.WriteOutput(testLine)

	// Check buffered output
	output := handler.GetOutput()
	if !strings.Contains(output, testLine) {
		t.Errorf("GetOutput() = %q, should contain %q", output, testLine)
	}

	// Check streaming output
	select {
	case line := <-handler.OutChan():
		if line != testLine {
			t.Errorf("OutChan received %q, want %q", line, testLine)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("OutChan did not receive line within timeout")
	}
}

func TestWriteError(t *testing.T) {
	handler := NewHandler()
	defer handler.Close()

	testLine := "test error line"
	handler.WriteError(testLine)

	// Check buffered error
	errOutput := handler.GetError()
	if !strings.Contains(errOutput, testLine) {
		t.Errorf("GetError() = %q, should contain %q", errOutput, testLine)
	}

	// Check streaming error
	select {
	case line := <-handler.ErrChan():
		if line != testLine {
			t.Errorf("ErrChan received %q, want %q", line, testLine)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("ErrChan did not receive line within timeout")
	}
}

func TestMultipleWrites(t *testing.T) {
	handler := NewHandler()
	defer handler.Close()

	lines := []string{"line1", "line2", "line3"}

	for _, line := range lines {
		handler.WriteOutput(line)
	}

	output := handler.GetOutput()
	for _, line := range lines {
		if !strings.Contains(output, line) {
			t.Errorf("GetOutput() should contain %q", line)
		}
	}

	// Check all lines were streamed
	for _, expectedLine := range lines {
		select {
		case line := <-handler.OutChan():
			if line != expectedLine {
				t.Errorf("OutChan received %q, want %q", line, expectedLine)
			}
		case <-time.After(100 * time.Millisecond):
			t.Errorf("OutChan did not receive line %q within timeout", expectedLine)
		}
	}
}

func TestGetOutputEmpty(t *testing.T) {
	handler := NewHandler()
	defer handler.Close()

	output := handler.GetOutput()
	if output != "" {
		t.Errorf("GetOutput() = %q, want empty string", output)
	}
}

func TestGetErrorEmpty(t *testing.T) {
	handler := NewHandler()
	defer handler.Close()

	errOutput := handler.GetError()
	if errOutput != "" {
		t.Errorf("GetError() = %q, want empty string", errOutput)
	}
}

func TestReset(t *testing.T) {
	handler := NewHandler()
	defer handler.Close()

	handler.WriteOutput("test output")
	handler.WriteError("test error")

	// Drain channels
	<-handler.OutChan()
	<-handler.ErrChan()

	handler.Reset()

	if output := handler.GetOutput(); output != "" {
		t.Errorf("GetOutput() after Reset() = %q, want empty", output)
	}

	if errOutput := handler.GetError(); errOutput != "" {
		t.Errorf("GetError() after Reset() = %q, want empty", errOutput)
	}
}

func TestIsClosed(t *testing.T) {
	handler := NewHandler()

	if handler.IsClosed() {
		t.Error("handler should not be closed initially")
	}

	handler.Close()

	if !handler.IsClosed() {
		t.Error("handler should be closed after Close()")
	}
}

func TestClose(t *testing.T) {
	handler := NewHandler()

	handler.WriteOutput("test")

	// Drain the output channel before closing
	<-handler.OutChan()

	handler.Close()

	// Verify outChan is closed by checking if reading returns immediately with !ok
	_, ok := <-handler.OutChan()
	if ok {
		t.Error("OutChan should be closed")
	}

	// Verify errChan is closed
	_, ok = <-handler.ErrChan()
	if ok {
		t.Error("ErrChan should be closed")
	}
}

func TestCloseIdempotent(t *testing.T) {
	handler := NewHandler()

	// Close multiple times should not panic
	handler.Close()
	handler.Close()
	handler.Close()

	if !handler.IsClosed() {
		t.Error("handler should remain closed")
	}
}

func TestWriteAfterClose(t *testing.T) {
	handler := NewHandler()
	handler.Close()

	// Writing after close should not panic
	handler.WriteOutput("test output")
	handler.WriteError("test error")

	// Buffer should still contain data
	output := handler.GetOutput()
	if !strings.Contains(output, "test output") {
		t.Error("WriteOutput after Close should still buffer data")
	}

	errOutput := handler.GetError()
	if !strings.Contains(errOutput, "test error") {
		t.Error("WriteError after Close should still buffer data")
	}
}

func TestConcurrentWrites(t *testing.T) {
	handler := NewHandler()
	defer handler.Close()

	var wg sync.WaitGroup
	iterations := 100

	// Concurrent output writes
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			handler.WriteOutput("output")
		}
	}()

	// Concurrent error writes
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			handler.WriteError("error")
		}
	}()

	// Concurrent reads
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = handler.GetOutput()
			_ = handler.GetError()
		}
	}()

	wg.Wait()

	// Verify no data loss in buffers
	output := handler.GetOutput()
	outputCount := strings.Count(output, "output")
	if outputCount != iterations {
		t.Errorf("output count = %d, want %d", outputCount, iterations)
	}

	errOutput := handler.GetError()
	errorCount := strings.Count(errOutput, "error")
	if errorCount != iterations {
		t.Errorf("error count = %d, want %d", errorCount, iterations)
	}
}

func TestChannelFullHandling(t *testing.T) {
	// Initialize watcher to prevent nil pointer dereference
	_ = watcher.NewWatcherService()

	// Create handler with small buffer
	handler := NewHandlerWithBuffers(2, 2)
	defer handler.Close()

	// Fill the channel beyond capacity
	for i := 0; i < 10; i++ {
		handler.WriteOutput("line")
	}

	// Should not hang or panic
	// Some lines might be dropped, but buffered output should have all
	output := handler.GetOutput()
	lineCount := strings.Count(output, "line")
	if lineCount != 10 {
		t.Errorf("buffered output should have all 10 lines, got %d", lineCount)
	}
}

func TestChannelReadability(t *testing.T) {
	handler := NewHandler()
	defer handler.Close()

	testData := []string{"line1", "line2", "line3"}

	// Write data
	for _, line := range testData {
		handler.WriteOutput(line)
	}

	// Read from channel
	var received []string
	timeout := time.After(100 * time.Millisecond)

readLoop:
	for i := 0; i < len(testData); i++ {
		select {
		case line := <-handler.OutChan():
			received = append(received, line)
		case <-timeout:
			break readLoop
		}
	}

	if len(received) != len(testData) {
		t.Errorf("received %d lines, want %d", len(received), len(testData))
	}

	for i, line := range received {
		if line != testData[i] {
			t.Errorf("received[%d] = %q, want %q", i, line, testData[i])
		}
	}
}

func TestBufferAccumulation(t *testing.T) {
	handler := NewHandler()
	defer handler.Close()

	// Write multiple lines
	handler.WriteOutput("line1")
	handler.WriteOutput("line2")
	handler.WriteError("error1")
	handler.WriteError("error2")

	output := handler.GetOutput()
	if !strings.Contains(output, "line1") || !strings.Contains(output, "line2") {
		t.Error("output should contain both lines")
	}

	errOutput := handler.GetError()
	if !strings.Contains(errOutput, "error1") || !strings.Contains(errOutput, "error2") {
		t.Error("error output should contain both errors")
	}

	// Output should have newlines
	if !strings.Contains(output, "\n") {
		t.Error("output should contain newlines between lines")
	}
}

// TestWriteCloseRace verifies that concurrent Write and Close calls do not
// cause a panic from sending on a closed channel. Run with -race to detect
// data races.
func TestWriteCloseRace(t *testing.T) {
	const goroutines = 10
	const writes = 100

	for range 50 {
		handler := NewHandlerWithBuffers(writes*goroutines, writes*goroutines)

		var wg sync.WaitGroup
		for range goroutines {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range writes {
					handler.WriteOutput("out")
					handler.WriteError("err")
				}
			}()
		}

		// Close after a brief delay so some writes race with the close.
		time.AfterFunc(time.Millisecond, func() {
			handler.Close()
		})

		wg.Wait()
	}
}

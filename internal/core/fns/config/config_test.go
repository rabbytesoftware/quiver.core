package config

import (
	"net/http"
	"testing"
	"time"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	if cfg.MaxMemorySize != DefaultMaxMemorySize {
		t.Errorf("Expected MaxMemorySize %d, got %d", DefaultMaxMemorySize, cfg.MaxMemorySize)
	}

	if cfg.BufferSize != DefaultBufferSize {
		t.Errorf("Expected BufferSize %d, got %d", DefaultBufferSize, cfg.BufferSize)
	}

	if cfg.HTTPClient == nil {
		t.Error("Expected HTTPClient to be initialized")
	}

	if cfg.HTTPClient.Timeout != DefaultHTTPTimeout {
		t.Errorf("Expected timeout %v, got %v", DefaultHTTPTimeout, cfg.HTTPClient.Timeout)
	}
}

func TestWithHTTPClient(t *testing.T) {
	customClient := &http.Client{Timeout: 60 * time.Second}
	cfg := Default()
	opt := WithHTTPClient(customClient)
	opt(&cfg)

	if cfg.HTTPClient != customClient {
		t.Error("Expected custom HTTP client to be set")
	}

	if cfg.HTTPClient.Timeout != 60*time.Second {
		t.Errorf("Expected timeout 60s, got %v", cfg.HTTPClient.Timeout)
	}
}

func TestWithHTTPClient_Nil(t *testing.T) {
	cfg := Default()
	originalClient := cfg.HTTPClient
	opt := WithHTTPClient(nil)
	opt(&cfg)

	if cfg.HTTPClient != originalClient {
		t.Error("Expected HTTPClient to remain unchanged when nil is passed")
	}
}

func TestWithMaxMemorySize(t *testing.T) {
	cfg := Default()
	opt := WithMaxMemorySize(50 * 1024 * 1024)
	opt(&cfg)

	if cfg.MaxMemorySize != 50*1024*1024 {
		t.Errorf("Expected MaxMemorySize 50MB, got %d", cfg.MaxMemorySize)
	}
}

func TestWithMaxMemorySize_Invalid(t *testing.T) {
	cfg := Default()
	originalSize := cfg.MaxMemorySize
	opt := WithMaxMemorySize(0)
	opt(&cfg)

	if cfg.MaxMemorySize != originalSize {
		t.Error("Expected MaxMemorySize to remain unchanged when 0 is passed")
	}

	opt = WithMaxMemorySize(-1)
	opt(&cfg)

	if cfg.MaxMemorySize != originalSize {
		t.Error("Expected MaxMemorySize to remain unchanged when negative is passed")
	}
}

func TestWithBufferSize(t *testing.T) {
	cfg := Default()
	opt := WithBufferSize(64 * 1024)
	opt(&cfg)

	if cfg.BufferSize != 64*1024 {
		t.Errorf("Expected BufferSize 64KB, got %d", cfg.BufferSize)
	}
}

func TestWithBufferSize_Invalid(t *testing.T) {
	cfg := Default()
	originalSize := cfg.BufferSize
	opt := WithBufferSize(0)
	opt(&cfg)

	if cfg.BufferSize != originalSize {
		t.Error("Expected BufferSize to remain unchanged when 0 is passed")
	}

	opt = WithBufferSize(-1)
	opt(&cfg)

	if cfg.BufferSize != originalSize {
		t.Error("Expected BufferSize to remain unchanged when negative is passed")
	}
}

func TestMultipleOptions(t *testing.T) {
	customClient := &http.Client{Timeout: 45 * time.Second}
	cfg := Default()

	opts := []Option{
		WithHTTPClient(customClient),
		WithMaxMemorySize(100 * 1024 * 1024),
		WithBufferSize(128 * 1024),
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	if cfg.HTTPClient != customClient {
		t.Error("Expected custom HTTP client")
	}

	if cfg.MaxMemorySize != 100*1024*1024 {
		t.Errorf("Expected MaxMemorySize 100MB, got %d", cfg.MaxMemorySize)
	}

	if cfg.BufferSize != 128*1024 {
		t.Errorf("Expected BufferSize 128KB, got %d", cfg.BufferSize)
	}
}

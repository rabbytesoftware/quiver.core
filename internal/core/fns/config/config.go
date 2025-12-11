package config

import (
	"net/http"
	"time"
)

const (
	DefaultMaxMemorySize = 20 * 1024 * 1024
	DefaultBufferSize    = 32 * 1024
	DefaultHTTPTimeout   = 30 * time.Second
)

type Config struct {
	MaxMemorySize int64
	HTTPClient    *http.Client
	BufferSize    int
}

func Default() Config {
	return Config{
		MaxMemorySize: DefaultMaxMemorySize,
		HTTPClient: &http.Client{
			Timeout: DefaultHTTPTimeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		BufferSize: DefaultBufferSize,
	}
}

type Option func(*Config)

func WithHTTPClient(client *http.Client) Option {
	return func(c *Config) {
		if client != nil {
			c.HTTPClient = client
		}
	}
}

func WithMaxMemorySize(size int64) Option {
	return func(c *Config) {
		if size > 0 {
			c.MaxMemorySize = size
		}
	}
}

func WithBufferSize(size int) Option {
	return func(c *Config) {
		if size > 0 {
			c.BufferSize = size
		}
	}
}

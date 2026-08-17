package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPredicate_ValidPort(t *testing.T) {
	testCases := []struct {
		name string
		port int
		want bool
	}{
		{name: "zero", port: 0, want: false},
		{name: "negative", port: -1, want: false},
		{name: "lower bound", port: 1, want: true},
		{name: "ephemeral start", port: 49152, want: true},
		{name: "upper bound", port: 65535, want: true},
		{name: "above range", port: 65536, want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, validPort(tc.port))
		})
	}
}

func TestPredicate_ValidDuration(t *testing.T) {
	testCases := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "seconds", value: "30s", want: true},
		{name: "minutes", value: "5m", want: true},
		{name: "hours", value: "720h", want: true},
		{name: "zero", value: "0s", want: false},
		{name: "negative", value: "-5m", want: false},
		{name: "unparseable", value: "banana", want: false},
		{name: "empty", value: "", want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, validDuration(tc.value))
		})
	}
}

func TestPredicate_ValidLogLevel(t *testing.T) {
	accepted := []string{"debug", "trace", "info", "warn", "warning", "error", "fatal", "panic"}
	for _, level := range accepted {
		t.Run(level, func(t *testing.T) {
			assert.True(t, validLogLevel(level))
		})
	}

	t.Run("uppercase accepted", func(t *testing.T) {
		assert.True(t, validLogLevel("INFO"))
	})

	rejected := []string{"waarn", "", "verbose", "notice"}
	for _, level := range rejected {
		t.Run("rejects "+level, func(t *testing.T) {
			assert.False(t, validLogLevel(level))
		})
	}
}

func TestPredicate_ValidHost(t *testing.T) {
	testCases := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "unix default", value: "unix://", want: true},
		{name: "unix explicit path", value: "unix:///tmp/quiver.sock", want: true},
		{name: "tcp all interfaces", value: "tcp://0.0.0.0:40257", want: true},
		{name: "tcp localhost", value: "tcp://127.0.0.1:9000", want: true},
		{name: "tcp missing port", value: "tcp://0.0.0.0", want: false},
		{name: "tcp empty authority", value: "tcp://", want: false},
		{name: "missing separator", value: "unix", want: false},
		{name: "unsupported scheme", value: "http://localhost:80", want: false},
		{name: "empty", value: "", want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, validHost(tc.value))
		})
	}
}

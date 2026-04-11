package api

import (
	"testing"

	"github.com/rabbytesoftware/quiver/internal/core/config"
	"github.com/stretchr/testify/assert"
)

func TestBuildAddr(t *testing.T) {
	cfg := config.API{Host: "0.0.0.0", Port: 40257}

	tests := []struct {
		name string
		host string
		port int
		want string
	}{
		{"uses config defaults when both zero", "", 0, "0.0.0.0:40257"},
		{"overrides host only", "127.0.0.1", 0, "127.0.0.1:40257"},
		{"overrides port only", "", 9090, "0.0.0.0:9090"},
		{"overrides both", "127.0.0.1", 9090, "127.0.0.1:9090"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, buildAddr(tt.host, tt.port, cfg))
		})
	}
}

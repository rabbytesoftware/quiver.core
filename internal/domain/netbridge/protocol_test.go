package netbridge

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProtocol(t *testing.T) {
	tests := []struct {
		protocol   Protocol
		wantString string
		wantValid  bool
		wantTCP    bool
		wantUDP    bool
		wantTCPUDP bool
	}{
		{ProtocolTCP, "tcp", true, true, false, false},
		{ProtocolUDP, "udp", true, false, true, false},
		{ProtocolTCPUDP, "tcp/udp", true, false, false, true},
		{Protocol("invalid"), "invalid", false, false, false, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.protocol), func(t *testing.T) {
			assert.Equal(t, tt.wantString, tt.protocol.String())
			assert.Equal(t, tt.wantValid, tt.protocol.IsValid())
			assert.Equal(t, tt.wantTCP, tt.protocol.IsTCP())
			assert.Equal(t, tt.wantUDP, tt.protocol.IsUDP())
			assert.Equal(t, tt.wantTCPUDP, tt.protocol.IsTCPUDP())
		})
	}
}

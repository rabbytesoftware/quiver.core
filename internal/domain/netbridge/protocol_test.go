package netbridge

import "testing"

func TestProtocol(t *testing.T) {
	tests := []struct {
		protocol    Protocol
		wantString  string
		wantValid   bool
		wantTCP     bool
		wantUDP     bool
		wantTCPUDP  bool
	}{
		{ProtocolTCP, "tcp", true, true, false, false},
		{ProtocolUDP, "udp", true, false, true, false},
		{ProtocolTCPUDP, "tcp/udp", true, false, false, true},
		{Protocol("invalid"), "invalid", false, false, false, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.protocol), func(t *testing.T) {
			if got := tt.protocol.String(); got != tt.wantString {
				t.Errorf("String() = %q, want %q", got, tt.wantString)
			}
			if got := tt.protocol.IsValid(); got != tt.wantValid {
				t.Errorf("IsValid() = %v, want %v", got, tt.wantValid)
			}
			if got := tt.protocol.IsTCP(); got != tt.wantTCP {
				t.Errorf("IsTCP() = %v, want %v", got, tt.wantTCP)
			}
			if got := tt.protocol.IsUDP(); got != tt.wantUDP {
				t.Errorf("IsUDP() = %v, want %v", got, tt.wantUDP)
			}
			if got := tt.protocol.IsTCPUDP(); got != tt.wantTCPUDP {
				t.Errorf("IsTCPUDP() = %v, want %v", got, tt.wantTCPUDP)
			}
		})
	}
}

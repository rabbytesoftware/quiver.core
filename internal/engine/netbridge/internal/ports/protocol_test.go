package ports

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProtocol_ConstantValues(
	t *testing.T,
) {
	assert.Equal(t, Protocol("tcp"), ProtocolTCP)
	assert.Equal(t, Protocol("udp"), ProtocolUDP)
	assert.Equal(t, Protocol("tcp/udp"), ProtocolTCPUDP)
}

func TestProtocol_StringRepresentation(
	t *testing.T,
) {
	assert.Equal(t, "tcp", string(ProtocolTCP))
	assert.Equal(t, "udp", string(ProtocolUDP))
	assert.Equal(t, "tcp/udp", string(ProtocolTCPUDP))
}

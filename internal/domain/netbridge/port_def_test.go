package netbridge

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPortDef_Validate_Valid(t *testing.T) {
	p := &PortDef{Name: "web", Protocol: ProtocolTCP, Default: 8080}
	assert.NoError(t, p.Validate())
}

func TestPortDef_Validate_NoDefault(t *testing.T) {
	p := &PortDef{Name: "web", Protocol: ProtocolUDP, Default: 0}
	assert.NoError(t, p.Validate())
}

func TestPortDef_Validate_EdgePorts(t *testing.T) {
	for _, d := range []int{MinPort, MaxPort} {
		p := &PortDef{Name: "web", Protocol: ProtocolTCPUDP, Default: d}
		assert.NoError(t, p.Validate(), "unexpected error for Default=%d", d)
	}
}

func TestPortDef_Validate_EmptyName(t *testing.T) {
	p := &PortDef{Name: "", Protocol: ProtocolTCP}
	require.Error(t, p.Validate())
}

func TestPortDef_Validate_DefaultBelowMin(t *testing.T) {
	p := &PortDef{Name: "web", Protocol: ProtocolTCP, Default: -1}
	require.Error(t, p.Validate())
}

func TestPortDef_Validate_DefaultAboveMax(t *testing.T) {
	p := &PortDef{Name: "web", Protocol: ProtocolTCP, Default: 70000}
	require.Error(t, p.Validate())
}

func TestPortDef_Validate_InvalidProtocol(t *testing.T) {
	p := &PortDef{Name: "web", Protocol: Protocol("invalid")}
	require.Error(t, p.Validate())
}

package netbridge

import "testing"

func TestPortDef_Validate_Valid(t *testing.T) {
	p := &PortDef{Name: "web", Protocol: ProtocolTCP, Default: 8080}
	if err := p.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPortDef_Validate_NoDefault(t *testing.T) {
	p := &PortDef{Name: "web", Protocol: ProtocolUDP, Default: 0}
	if err := p.Validate(); err != nil {
		t.Errorf("unexpected error for Default=0: %v", err)
	}
}

func TestPortDef_Validate_EdgePorts(t *testing.T) {
	for _, d := range []int{MinPort, MaxPort} {
		p := &PortDef{Name: "web", Protocol: ProtocolTCPUDP, Default: d}
		if err := p.Validate(); err != nil {
			t.Errorf("unexpected error for Default=%d: %v", d, err)
		}
	}
}

func TestPortDef_Validate_EmptyName(t *testing.T) {
	p := &PortDef{Name: "", Protocol: ProtocolTCP}
	if err := p.Validate(); err == nil {
		t.Error("expected error for empty name")
	}
}

func TestPortDef_Validate_DefaultBelowMin(t *testing.T) {
	p := &PortDef{Name: "web", Protocol: ProtocolTCP, Default: -1}
	if err := p.Validate(); err == nil {
		t.Error("expected error for Default < MinPort")
	}
}

func TestPortDef_Validate_DefaultAboveMax(t *testing.T) {
	p := &PortDef{Name: "web", Protocol: ProtocolTCP, Default: 70000}
	if err := p.Validate(); err == nil {
		t.Error("expected error for Default > MaxPort")
	}
}

func TestPortDef_Validate_InvalidProtocol(t *testing.T) {
	p := &PortDef{Name: "web", Protocol: Protocol("invalid")}
	if err := p.Validate(); err == nil {
		t.Error("expected error for invalid protocol")
	}
}

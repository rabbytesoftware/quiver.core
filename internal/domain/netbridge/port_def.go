package netbridge

import "fmt"

const (
	MinPort = 1
	MaxPort = 65535
)

type PortDef struct {
	Name     string   `yaml:"name"     json:"name"`
	Protocol Protocol `yaml:"protocol" json:"protocol"`
	Default  int      `yaml:"default"  json:"default"`
	Required bool     `yaml:"required" json:"required"`
}

func (p *PortDef) Validate() error {
	if p.Name == "" {
		return fmt.Errorf("port name cannot be empty")
	}

	if p.Default != 0 && (p.Default < MinPort || p.Default > MaxPort) {
		return fmt.Errorf(
			"port default %d out of range [%d, %d]",
			p.Default,
			MinPort,
			MaxPort,
		)
	}

	if !p.Protocol.IsValid() {
		return fmt.Errorf("invalid protocol: %s", p.Protocol)
	}

	return nil
}

package netbridge

const (
	MinPort = 1
	MaxPort = 65535
)

type PortDef struct {
	Name     string
	Protocol Protocol
	Default  int
	Required bool
}

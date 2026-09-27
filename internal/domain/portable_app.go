package domain

type PortableApp struct {
	Name  string `json:"name"           yaml:"name"`
	Entry string `json:"entry"          yaml:"entry"`
	Icon  string `json:"icon,omitempty" yaml:"icon,omitempty"`
}

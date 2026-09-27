package domain

const ExposeAuto = "auto"

type Expose struct {
	CLI     []ExposeEntry `yaml:"cli"     json:"cli,omitempty"`
	Desktop []ExposeEntry `yaml:"desktop" json:"desktop,omitempty"`
}

func (e Expose) IsEmpty() bool {
	return len(e.CLI) == 0 && len(e.Desktop) == 0
}

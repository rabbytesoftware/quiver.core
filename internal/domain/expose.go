package domain

const ExposeAuto = "auto"

type ExposeKind string

const (
	ExposeKindCLI     ExposeKind = "cli"
	ExposeKindDesktop ExposeKind = "desktop"
)

type Expose struct {
	CLI     []ExposeEntry `yaml:"cli"     json:"cli,omitempty"`
	Desktop []ExposeEntry `yaml:"desktop" json:"desktop,omitempty"`
}

func (e Expose) IsEmpty() bool {
	return len(e.CLI) == 0 && len(e.Desktop) == 0
}

type ExposeEntry struct {
	Name       string   `yaml:"name"       json:"name"`
	Path       string   `yaml:"path"       json:"path"`
	Icon       string   `yaml:"icon"       json:"icon,omitempty"`
	Categories []string `yaml:"categories" json:"categories,omitempty"`
}

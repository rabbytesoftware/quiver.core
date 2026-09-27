package domain

type ExposeEntry struct {
	Name       string   `yaml:"name"       json:"name"`
	Path       string   `yaml:"path"       json:"path"`
	Icon       string   `yaml:"icon"       json:"icon,omitempty"`
	Categories []string `yaml:"categories" json:"categories,omitempty"`
}

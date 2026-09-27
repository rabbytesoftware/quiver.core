package forge

type generator struct {
	Name       string   `yaml:"name"`
	Confidence string   `yaml:"confidence"`
	Warnings   []string `yaml:"warnings,omitempty"`
}

package forge

type metadata struct {
	Name        string    `yaml:"name"`
	Description string    `yaml:"description,omitempty"`
	URL         string    `yaml:"url,omitempty"`
	Media       media     `yaml:"media,omitempty"`
	Generator   generator `yaml:"generator"`
}

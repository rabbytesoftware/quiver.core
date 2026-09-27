package forge

type document struct {
	Schema   string            `yaml:"schema"`
	Metadata metadata          `yaml:"metadata"`
	Targets  map[string]target `yaml:"targets"`
}

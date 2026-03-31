package v0

type quiverV0 struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	URL         string   `yaml:"url"`
	Maintainers []string `yaml:"maintainers"`
	Tags        []string `yaml:"tags"`
	Media       mediaV0  `yaml:"media"`
	Arrows      []string `yaml:"arrows"`
}

type mediaV0 struct {
	Icon   string `yaml:"icon"`
	Banner string `yaml:"banner"`
}

package runtime

type ExposeResult struct {
	Entries []ExposedEntry  `yaml:"entries" json:"entries"`
	Refused []ExposeRefusal `yaml:"refused" json:"refused"`
}

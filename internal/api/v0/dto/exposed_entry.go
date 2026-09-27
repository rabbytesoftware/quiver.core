package dto

type ExposedEntryDTO struct {
	Kind     string `json:"kind" yaml:"kind"`
	Name     string `json:"name" yaml:"name"`
	Target   string `json:"target" yaml:"target"`
	Location string `json:"location" yaml:"location"`
}

package models

// RawMethod is a developer-defined action group on an Arrow, with string
// state names (not yet validated as domain.ArrowState) and raw steps.
type RawMethod struct {
	AvailableIn []string  `yaml:"available_in"`
	Steps       []RawStep `yaml:"steps"`
}

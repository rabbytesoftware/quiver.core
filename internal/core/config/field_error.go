package config

// FieldError names a configuration field that failed validation and explains
// why, using the dotted path the configuration API addresses fields by.
type FieldError struct {
	Key     string `json:"key"     yaml:"key"`
	Message string `json:"message" yaml:"message"`
}

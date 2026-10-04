package dto

type ConsoleFlagDTO struct {
	Name       string `json:"name" yaml:"name"`
	Shorthand  string `json:"shorthand" yaml:"shorthand"`
	Usage      string `json:"usage" yaml:"usage"`
	TakesValue bool   `json:"takes_value" yaml:"takes_value"`
}
